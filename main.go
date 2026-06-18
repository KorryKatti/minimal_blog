package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// =====================
// Constants & Types
// =====================

const keyServerAddr = "serverAddr"

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type BlogPost struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

type Comment struct {
	ID        int       `json:"id"`
	PostID    int       `json:"post_id"`
	ParentID  *int      `json:"parent_id,omitempty"` // pointer so omitempty works for NULL
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Replies   []Comment `json:"replies,omitempty"` // populated server-side, not stored
}

type VoteRequest struct {
	Value int `json:"value"` // 1 or -1
}

// =====================
// Global State
// =====================

var (
	db *sql.DB
)

// =====================
// Main
// =====================

func main() {
	fmt.Println("Hello, minimal blog")

	var err error
	db, err = sql.Open("sqlite3", "./blog.db")
	if err != nil {
		fmt.Printf("failed to open db: %s\n", err)
		return
	}
	defer db.Close()

	if err := initDB(); err != nil {
		fmt.Printf("failed to init db: %s\n", err)
		return
	}

	mux := http.NewServeMux()

	// basic routes
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/hello", getHello)

	// auth routes
	mux.HandleFunc("/signup", signupHandler)
	mux.HandleFunc("/signin", signinHandler)

	// public post routes
	mux.HandleFunc("/posts", postHandler)
	mux.HandleFunc("/posts/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/posts/")
		parts := strings.Split(path, "/")
		if len(parts) == 2 && parts[1] == "comments" {
			commentsHandler(w, r)
			return
		}
		postByIDHandler(w, r)
	})

	// protected post routes
	mux.HandleFunc("/api/posts", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			createPost(w, r)
			return
		}
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
	}))

	mux.HandleFunc("/api/posts/", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/posts/")
		parts := strings.Split(path, "/")
		if len(parts) < 1 {
			http.Error(w, "invalid path", http.StatusBadRequest) // prolly will never happen
			return
		}
		id, err := strconv.Atoi(parts[0])
		if err != nil {
			http.Error(w, "invalid post ID", http.StatusBadRequest)
			return
		}

		if len(parts) == 1 {
			if r.Method == http.MethodDelete {
				deletePost(w, r, id)
			}
			return
		}

		switch parts[1] {
		case "comments":
			if r.Method == http.MethodPost {
				createComment(w, r, id)
			}
		case "vote":
			if r.Method == http.MethodPost {
				votePost(w, r, id)
			}
		}
	}))

	ctx, cancelCtx := context.WithCancel(context.Background())

	server := &http.Server{
		Addr:    ":3456",
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
// Database
// =====================

func initDB() error {
	schema := `
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

	CREATE TABLE IF NOT EXISTS sessions(
		token TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		expires_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER NOT NULL,
		parent_id INTEGER, -- NULL = top-level comment, set = reply to another comment
		author TEXT NOT NULL,
		body TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
		FOREIGN KEY (parent_id) REFERENCES comments(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS votes (
		post_id INTEGER NOT NULL,
		username TEXT NOT NULL,
		value INTEGER NOT NULL CHECK(value IN(-1,1)),
		PRIMARY KEY(post_id,username),-- one vote per user per post
		FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE
	);

	`
	_, err := db.Exec(schema)
	return err
}

// =====================
// Basic Handlers
// =====================

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

// =====================
// Auth
// =====================

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

	hashedPw, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	res, err := db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", u.Username, string(hashedPw))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "user already exists", http.StatusBadRequest)
			return
		}
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

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

	var storedHash string
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", u.Username).Scan(&storedHash)
	if err == sql.ErrNoRows {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(u.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(tokenBytes)

	_, err = db.Exec(
		"INSERT INTO sessions (token, username, expires_at) VALUES (?, ?, datetime('now', '+1 day'))",
		token, u.Username,
	)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing auth token", http.StatusUnauthorized)
			return
		}

		var token string
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			http.Error(w, "invalid auth header format", http.StatusUnauthorized)
			return
		}

		var username string
		err := db.QueryRow(
			"SELECT username FROM sessions WHERE token = ? AND expires_at > datetime('now')",
			token,
		).Scan(&username)
		if err == sql.ErrNoRows {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), "username", username)
		next(w, r.WithContext(ctx))
	}
}

// =====================
// Posts
// =====================

func postHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		createPost(w, r)
	case http.MethodGet:
		listPosts(w, r)
	default:
		http.Error(w, "only Post or Get allowed", http.StatusMethodNotAllowed)
	}
}

func postByIDHandler(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/posts/")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid post id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getPost(w, r, id)
	case http.MethodDelete:
		deletePost(w, r, id)
	default:
		http.Error(w, "only get or delete allowed", http.StatusMethodNotAllowed)
	}
}

func createPost(w http.ResponseWriter, r *http.Request) {
	author, ok := r.Context().Value("username").(string)
	if !ok || author == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.Body == "" {
		http.Error(w, "title and body are required", http.StatusBadRequest)
		return
	}

	res, err := db.Exec(
		"INSERT INTO posts (title, body, author) VALUES (?, ?, ?)",
		req.Title, req.Body, author,
	)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	id, _ := res.LastInsertId()

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
	type PostWithVotes struct {
		BlogPost
		Upvotes   int `json:"upvotes"`
		Downvotes int `json:"downvotes"`
		Score     int `json:"score"`
	}

	rows, err := db.Query(`
		SELECT 
			p.id, p.title, p.body, p.author, p.created_at,
			COALESCE(SUM(CASE WHEN v.value = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN v.value = -1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(v.value), 0)
		FROM posts p
		LEFT JOIN votes v ON p.id = v.post_id
		GROUP BY p.id
		ORDER BY p.created_at DESC
	`)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	posts := make([]PostWithVotes, 0)
	for rows.Next() {
		var p PostWithVotes
		if err := rows.Scan(&p.ID, &p.Title, &p.Body, &p.Author, &p.CreatedAt, &p.Upvotes, &p.Downvotes, &p.Score); err != nil {
			continue
		}
		posts = append(posts, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posts)
}

func getPost(w http.ResponseWriter, r *http.Request, id int) {
	type PostWithVotes struct {
		BlogPost
		Upvotes   int `json:"upvotes"`
		Downvotes int `json:"downvotes"`
		Score     int `json:"score"`
	}

	var p PostWithVotes
	err := db.QueryRow(`
		SELECT 
			p.id, p.title, p.body, p.author, p.created_at,
			COALESCE(SUM(CASE WHEN v.value = 1 THEN 1 ELSE 0 END), 0) as upvotes,
			COALESCE(SUM(CASE WHEN v.value = -1 THEN 1 ELSE 0 END), 0) as downvotes,
			COALESCE(SUM(v.value), 0) as score
		FROM posts p
		LEFT JOIN votes v ON p.id = v.post_id
		WHERE p.id = ?
		GROUP BY p.id
	`, id).Scan(&p.ID, &p.Title, &p.Body, &p.Author, &p.CreatedAt, &p.Upvotes, &p.Downvotes, &p.Score)

	if err == sql.ErrNoRows {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
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

func createComment(w http.ResponseWriter, r *http.Request, postID int) {
	author, ok := r.Context().Value("username").(string)

	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Body     string `json:"body"`
		ParentID *int   `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Body == "" {
		http.Error(w, "body required", http.StatusBadRequest)
		return
	}

	res, err := db.Exec(
		"INSERT INTO comments (post_id, parent_id,author,body) VALUES (?,?,?,?)",
		postID, req.ParentID, author, req.Body,
	)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	id, _ := res.LastInsertId()
	var c Comment
	err = db.QueryRow(
		"SELECT id,post_id,parent_id,author,body,created_at FROM comments WHERE id=?",
		id,
	).Scan(&c.ID, &c.PostID, &c.ParentID, &c.Author, &c.Body, &c.CreatedAt)

	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}

func commentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "only get ", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/posts/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "comments" { //surely there is a better way
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	postID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "invalid post ID", http.StatusBadRequest)
		return
	}

	// fetch all comments for this post
	rows, err := db.Query(
		"SELECT id,post_id,parent_id,author,body,created_at FROM comments WHERE post_id=?",
		postID,
	)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// load all comments in memory
	// sounds bad
	allComments := make(map[int]Comment)
	var topLevel []Comment

	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.ParentID, &c.Author, &c.Body, &c.CreatedAt); err != nil {
			continue
		}
		allComments[c.ID] = c
	}

	// build tree to assign replies to parent
	for _, c := range allComments {
		if c.ParentID == nil {
			topLevel = append(topLevel, c)
		} else {
			// has parent = reply, add to parent's Replies slice
			parent := allComments[*c.ParentID]
			parent.Replies = append(parent.Replies, c)
			allComments[*c.ParentID] = parent // update in map
		}
	}
	// now update topLevel with modified parents that have reploes populated
	for i:=range topLevel{
		topLevel[i]=allComments[topLevel[i].ID]
	}
	w.Header().Set("Content-Type","application/json")
	json.NewEncoder(w).Encode(topLevel)
}

func votePost(w http.ResponseWriter,r *http.Request, postID int){
	author,ok := r.Context().Value("username").(string)
	if !ok {
		http.Error(w,"unahotizied",http.StatusUnauthorized)
		return
	}

	var req VoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err!=nil {
		http.Error(w,"invalid body",http.StatusBadRequest)
		return
	}
	if req.Value != 1 && req.Value!=-1{
		http.Error(w,"value must be 1 or -1",http.StatusBadRequest)
		return
	}

	_,err := db.Exec(
		"INSERT OR REPLACE INTO votes (post_id,username,value) VALUES (?,?,?)",
		postID,author,req.Value,
	)
	if err!=nil{
		http.Error(w,"database error",http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	io.WriteString(w,"voted\n")
}
