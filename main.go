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
	// "sync"     // sync access to shared memory // idk what that means yet
	"time"
	"strconv"
	"database/sql" //obv
	_ "github.com/mattn/go-sqlite3" // the _ means import for side effects
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

var (
	db *sql.DB // database connection pool
)

func main() {
	fmt.Println("Hello, minimal blog")


	// open sqlite file , create if not exists
	var err error
	db,err = sql.Open("sqlite3","./blog.db")
	
	if err!=nil{
		fmt.Printf("failed to open db: %s\n",err)
		return
	}
	defer db.Close()

	// creat table if doent exist
	if err:= initDB(); err!=nil{
		fmt.Printf("failed to init db: %s\n",err)
		return
	}

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
func initDB() error {
	schema:=`
	CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		password TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		body TEXT NOT NULL,
		author TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_,err := db.Exec(schema)
	return err
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

func createPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title  string `json:"title"`
		Body   string `json:"body"`
		Author string `json:"author"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.Body == "" || req.Author == "" {
		http.Error(w, "title, body, and author are required", http.StatusBadRequest)
		return
	}

	// Exec for INSERT (no rows returned)
	res, err := db.Exec(
		"INSERT INTO posts (title, body, author) VALUES (?, ?, ?)",
		req.Title, req.Body, req.Author,
	)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// get the auto-generated ID back
	id, _ := res.LastInsertId()

	// fetch the full row to return created_at (auto-set by SQLite)
	var post BlogPost
	err = db.QueryRow(
		"SELECT id, title, body, author, created_at FROM posts WHERE id = ?",
		id,
	).Scan(&post.ID, &post.Title, &post.Body, &post.Author, &post.CreatedAt)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(post)
}

func listPosts(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, title, body, author, created_at FROM posts")
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close() // always close rows when done, or connection leaks

	var posts []BlogPost
	for rows.Next() {
		var p BlogPost
		if err := rows.Scan(&p.ID, &p.Title, &p.Body, &p.Author, &p.CreatedAt); err != nil {
			continue // skip bad rows
		}
		posts = append(posts, p)
	}

	w.Header().Set("Content-Type", "application/json")
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



func deletePost(w http.ResponseWriter, r *http.Request, id int) {
	res, err := db.Exec("DELETE FROM posts WHERE id = ?", id)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getPost(w http.ResponseWriter, r *http.Request, id int) {
	var post BlogPost
	err := db.QueryRow(
		"SELECT id, title, body, author, created_at FROM posts WHERE id = ?",
		id,
	).Scan(&post.ID, &post.Title, &post.Body, &post.Author, &post.CreatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
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
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	res, err := db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", u.Username, u.Password)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "user already exists", http.StatusBadRequest)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// check if actually inserted (should be 1 row)
	rows, _ := res.RowsAffected()
	if rows == 0 {
		http.Error(w, "user already exists", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	io.WriteString(w, "user created successfully\n")
}

func signinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var storedPassword string
	// QueryRow for single result, Scan puts values into your variables
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", u.Username).Scan(&storedPassword)
	if err == sql.ErrNoRows || storedPassword != u.Password {
		http.Error(w, "invalid credentials", http.StatusUnauthorized) // 401, not 400
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	io.WriteString(w, "user signed in successfully\n")
}