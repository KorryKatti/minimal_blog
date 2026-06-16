package main

import (
	"context"       // for request and server contexts
	"encoding/json" // for json stuff
	"strings"
	// "errors" // obv
	"fmt" // for printing
	"io"
	"net"      // for networok addresses
	"net/http" // for http server
	"sync"     // sync access to shared memory // idk what that means yet
	"time"
	"strconv"
)

const keyServerAddr = "serverAddr"

type User struct {
	Username string `json:"username"` // name type tag ( tag is like a label for the field so that the json encoder knows what to name the field when encoding)
	Password string `json:"password"` // same thing here	
}

type BlogPost struct {
	ID int `json:"id"` // unique number for each post
	Title string `json:"title"`
	Body string `json:"body"`
	Author string `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// memory map
var ( // var is used for global variables
	users = make(map[string]string) // map to store users , format is <username,password>
	usersMu sync.Mutex // mutex to sync access to users map , makes it not have race conditions
	blogPosts = make(map[int]BlogPost) // id -> post
	blogPostsMu sync.Mutex // different mutex so users and posts dont block each other
	nextPostID = 1 // auto incrememnt id
)

func main() {
	fmt.Println("Hello, minimal blog")

	mux := http.NewServeMux()
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/hello", getHello)
	mux.HandleFunc("/signup", signupHandler)
	mux.HandleFunc("/signin", signinHandler)
	mux.HandleFunc("/posts",postHandler) // handles bboth create and list
	// :id means "catch anything after /posts/" — we parse it manually
	mux.HandleFunc("/posts/", postByIDHandler)

	ctx, cancelCtx := context.WithCancel(context.Background())

	server := &http.Server{
		Addr: ":3456",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			ctx = context.WithValue(ctx, keyServerAddr, l.Addr().String())
			return ctx
		},
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %s\n", err)
		}
		cancelCtx()
	}()

	<-ctx.Done()
	fmt.Println("server stopped")
}

// =====================

func postHandler(w http.ResponseWriter,r *http.Request){
	switch r.Method {
	case http.MethodPost:
		createPost(w,r)
	case http.MethodGet:
		listPosts(w,r)
	default:
		http.Error(w,"only Post or Get allowed",http.StatusMethodNotAllowed)
	}
}

func createPost(w http.ResponseWriter,r *http.Request){
	// decode request body into a temporary struct ( client not trusted to send id and createdat
	var req struct {
		Title string `json:"title"`
		Body string `json:"body"`
		Author string `json:"author"`
	}
	if err:= json.NewDecoder(r.Body).Decode(&req); err!=nil{
		http.Error(w,"invalid request body",http.StatusBadRequest)
		return
	}
	// basic validaiton
	if req.Title == "" || req.Body == "" || req.Author == "" {
		http.Error(w,"title,body and author are required",http.StatusBadRequest)
		return
	}

	blogPostsMu.Lock()
	defer blogPostsMu.Unlock()

	post := BlogPost {
		ID: nextPostID,
		Title: req.Title,
		Body: req.Body,
		Author: req.Author,
		CreatedAt: time.Now(),
	}

	blogPosts[post.ID]=post
	nextPostID++

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(post) // send back created post with id and timestamp
}

func listPosts(w http.ResponseWriter,r *http.Request){
	blogPostsMu.Lock()
	defer blogPostsMu.Unlock()

	// i will make a db
	posts:=make([]BlogPost,0,len(blogPosts))// copy map values into sliec so we can return them as json array
	for _,post := range blogPosts{
		posts = append(posts,post)
	}
	w.Header().Set("Content-Type","application.json")
	json.NewEncoder(w).Encode(posts)
}


func postByIDHandler(w http.ResponseWriter,r *http.Request){
	// /post/1 -> need to extract 1
	idStr:=strings.TrimPrefix(r.URL.Path,"/posts/")

	// conver string to integer
	id,err := strconv.Atoi(idStr)
	if err!=nil{
		http.Error(w,"invalid post id",http.StatusBadRequest)
		return
	}

	switch r.Method{
	case http.MethodGet:
		getPost(w,r,id)
	case http.MethodDelete:
		deletePost(w,r,id)
	default:
		http.Error(w,"only get or delete allowed",http.StatusMethodNotAllowed)
	}
}

func getPost(w http.ResponseWriter,r *http.Request,id int){
	blogPostsMu.Lock()
	defer blogPostsMu.Unlock()

	post,exists:=blogPosts[id]
	if !exists{
		http.Error(w,"post not found",http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type","application/json")
	json.NewEncoder(w).Encode(post)
}

func deletePost(w http.ResponseWriter,r *http.Request,id int){
	blogPostsMu.Lock()
	defer blogPostsMu.Unlock()

	if _,exists := blogPosts[id]; !exists{
		http.Error(w,"post not found",http.StatusNotFound)
		return
	}

	delete(blogPosts,id)
	w.WriteHeader(http.StatusNoContent)
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fmt.Printf("server addr: %s\n", ctx.Value(keyServerAddr))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("couldn't read body: %s\n", err)
	}

	fmt.Printf("request body: %s\n", string(body))
	io.WriteString(w, "Hello from minimal blog!\n")
}

func getHello(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fmt.Printf("server addr: %s\n", ctx.Value(keyServerAddr))

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("couldn't read body: %s\n", err)
	}

	fmt.Printf("request body: %s\n", string(body))
	io.WriteString(w, "Hello from minimal blog!\n")
}

func signupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed, ONLY POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	usersMu.Lock()         // 🔑 fix: was usersMu.lock() → Go is case-sensitive
	defer usersMu.Unlock() // defer unlock

	if _, exists := users[u.Username]; exists {
		http.Error(w, "user already exists", http.StatusBadRequest)
		return
	}

	users[u.Username] = u.Password
	w.WriteHeader(http.StatusCreated)
	io.WriteString(w, "user created successfully\n")
}

func signinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Only POST allowed")
		return
	}

	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	usersMu.Lock()
	defer usersMu.Unlock()

	pw, exists := users[u.Username]
	if !exists || pw != u.Password {
		http.Error(w, "invalid credentials", http.StatusBadRequest)
		return
	}

	io.WriteString(w, "user signed in successfully\n")
}