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
	"sync"
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

// standard JSON response wrapper
type Response struct {
	Data  interface{} `json:"data,omitempty"`  // success payload (omitempty = skip if nil)
	Error string      `json:"error,omitempty"` // error message (omitempty = skip if empty)
}

// writeJSON sends a JSON response with proper Content-Type
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Data: data})
}

// writeError sends a consistent error response
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{Error: message})
}

// corsMiddleware wraps handlers to add CORS headers
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// preflight request — browser sends OPTIONS before actual request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}



// =====================
// Rate Limiting
// =====================
type rateLimiter struct {
	count int // how many requests so far
	resetAt time.Time
	mu sync.Mutex
}

// global map of ip->limiter
var (
	rateLimiters = make(map[string]*rateLimiter)
	rateLimitersMu sync.Mutex
)

const (
	rateLimitWindow = time.Minute // 1 minute window
	rateLimitMax = 60 // max 60 requests per minute per ip
)

// ratelimitmiddleware enforces rate limits
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc{
	return func(w http.ResponseWriter,r *http.Request){
		// get client ip
		ip:=r.RemoteAddr
		// strip port from remote address
		if host,_,err := net.SplitHostPort(ip);err==nil{
			ip=host
		}
		// lock the global map to get/crate this ip's limiter
		rateLimitersMu.Lock()
		lim,exists := rateLimiters[ip]
		if !exists {
			lim = &rateLimiter{resetAt: time.Now().Add(rateLimitWindow)}
			rateLimiters[ip]=lim
		}
		rateLimitersMu.Unlock()

		// now lock just this IP's limiter
		lim.mu.Lock()
		defer lim.mu.Unlock()

		// check if window expired-> reset counter
		now := time.Now()
		if now.After(lim.resetAt){
			lim.count=0
			lim.resetAt=now.Add(rateLimitWindow)
		}
		// increment and check
		lim.count++
		if lim.count > rateLimitMax{
			writeError(w,http.StatusTooManyRequests,"rate limit exceeded try again")
			return
		}
		next(w,r)
	}
}

func validateUsername(username string) error {
	if username == ""{
		return fmt.Errorf("username is required")
	}
	if len(username)<3 || len(username)>30{
		return fmt.Errorf("username must be between 3-30 characters")
	}
		for _, ch := range username {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			return fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
		}
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}
	if len(password) > 100 {
		return fmt.Errorf("password too long")
	}
	return nil
}

func sanitizeString(s string) string {
	// strings.NewReplacer replaces multiple substrings in one pass
	// It's efficient — goes through the string once, not 4 times
	replacer := strings.NewReplacer(
		"&", "&amp;",   // must be first! otherwise & in others gets double-escaped
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return replacer.Replace(s)
}

// validatePost checks title and body for valid content
func validatePost(title, body string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title is required")
	}
	if len(title) > 200 {
		return fmt.Errorf("title too long (max 200 characters)")
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("body is required")
	}
	if len(body) > 100000 {
		return fmt.Errorf("body too long (max 100000 characters)")
	}
	return nil
}

// validateComment checks comment body
func validateComment(body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("comment body is required")
	}
	if len(body) > 2000 {
		return fmt.Errorf("comment too long (max 2000 characters)")
	}
	return nil
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


	// public routes — Rate limit → CORS → handler
	mux.HandleFunc("/", rateLimitMiddleware(corsMiddleware(getRoot)))
	mux.HandleFunc("/hello", rateLimitMiddleware(corsMiddleware(getHello)))
	mux.HandleFunc("/signup", rateLimitMiddleware(corsMiddleware(signupHandler)))
	mux.HandleFunc("/signin", rateLimitMiddleware(corsMiddleware(signinHandler)))
	mux.HandleFunc("/posts", rateLimitMiddleware(corsMiddleware(listPosts)))
	mux.HandleFunc("/posts/", rateLimitMiddleware(corsMiddleware(postByIDHandler)))

	// protected routes — Rate limit → CORS → Auth → handler
	mux.HandleFunc("/api/posts", rateLimitMiddleware(corsMiddleware(authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			createPost(w, r)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "only POST allowed")
	}))))

	mux.HandleFunc("/api/posts/", rateLimitMiddleware(corsMiddleware(authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/posts/")
		parts := strings.Split(path, "/")
		if len(parts) < 1 {
			writeError(w, http.StatusBadRequest, "invalid path")
			return
		}
		id, err := strconv.Atoi(parts[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid post ID")
			return
		}

		if len(parts) == 1 {
			if r.Method == http.MethodDelete {
				deletePost(w, r, id)
				return
			}
			writeError(w, http.StatusMethodNotAllowed, "only DELETE allowed")
			return
		}

		switch parts[1] {
		case "comments":
			if r.Method == http.MethodPost {
				createComment(w, r, id)
				return
			}
			writeError(w, http.StatusMethodNotAllowed, "only POST allowed")
		case "vote":
			if r.Method == http.MethodPost {
				votePost(w, r, id)
				return
			}
			writeError(w, http.StatusMethodNotAllowed, "only POST allowed")
		default:
			writeError(w, http.StatusNotFound, "unknown endpoint")
		}
	}))))

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
		writeError(w, http.StatusMethodNotAllowed, "only POST allowed")
		return
	}

	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	u.Username = strings.TrimSpace(u.Username)
	u.Password = strings.TrimSpace(u.Password)

	if err := validateUsername(u.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePassword(u.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hashedPw, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	_, err = db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", u.Username, string(hashedPw))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusBadRequest, "user already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "user created"})
}

func signinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "only POST allowed")
		return
	}

	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	u.Username = strings.TrimSpace(u.Username)
	u.Password = strings.TrimSpace(u.Password)

	if err := validateUsername(u.Username); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	var storedHash string
	err := db.QueryRow("SELECT password FROM users WHERE username = ?", u.Username).Scan(&storedHash)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(u.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// generate token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	_, err = db.Exec(
		"INSERT INTO sessions (token, username, expires_at) VALUES (?, ?, datetime('now', '+1 day'))",
		token, u.Username,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, "missing auth token")
			return
		}

		var token string
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			writeError(w, http.StatusUnauthorized, "invalid auth header format")
			return
		}

		var username string
		err := db.QueryRow(
			"SELECT username FROM sessions WHERE token = ? AND expires_at > datetime('now')",
			token,
		).Scan(&username)
		if err == sql.ErrNoRows {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
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
		writeError(w, http.StatusMethodNotAllowed, "only POST or GET allowed")
	}
}

func postByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/posts/")
	parts := strings.Split(path, "/")

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid post id")
		return
	}

	if len(parts) > 1 && parts[1] == "comments" {
		if r.Method == http.MethodGet {
			commentsHandler(w, r, id)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "only GET allowed")
		return
	}

	switch r.Method {
	case http.MethodGet:
		getPost(w, r, id)
	case http.MethodDelete:
		deletePost(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "only get or delete allowed")
	}
}

func createPost(w http.ResponseWriter, r *http.Request) {
	author, ok := r.Context().Value("username").(string)
	if !ok || author == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)

	if err := validatePost(req.Title, req.Body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	req.Title = sanitizeString(req.Title)
	req.Body = sanitizeString(req.Body)

	res, err := db.Exec(
		"INSERT INTO posts (title, body, author) VALUES (?, ?, ?)",
		req.Title, req.Body, author,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	id, _ := res.LastInsertId()

	var post BlogPost
	err = db.QueryRow(
		"SELECT id, title, body, author, created_at FROM posts WHERE id = ?",
		id,
	).Scan(&post.ID, &post.Title, &post.Body, &post.Author, &post.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, post)
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
		writeError(w, http.StatusInternalServerError, "database error")
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

	writeJSON(w, http.StatusOK, posts)
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
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

func deletePost(w http.ResponseWriter, r *http.Request, id int) {
	res, err := db.Exec("DELETE FROM posts WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

func createComment(w http.ResponseWriter, r *http.Request, postID int) {
	author, ok := r.Context().Value("username").(string)

	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Body     string `json:"body"`
		ParentID *int   `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	req.Body = strings.TrimSpace(req.Body)

	if err := validateComment(req.Body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	req.Body = sanitizeString(req.Body)

	res, err := db.Exec(
		"INSERT INTO comments (post_id, parent_id, author, body) VALUES (?, ?, ?, ?)",
		postID, req.ParentID, author, req.Body,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	id, _ := res.LastInsertId()
	var c Comment
	err = db.QueryRow(
		"SELECT id,post_id,parent_id,author,body,created_at FROM comments WHERE id=?",
		id,
	).Scan(&c.ID, &c.PostID, &c.ParentID, &c.Author, &c.Body, &c.CreatedAt)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, c)
}

func commentsHandler(w http.ResponseWriter, r *http.Request, postID int) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "only GET allowed")
		return
	}

	// fetch all comments for this post
	rows, err := db.Query(
		"SELECT id,post_id,parent_id,author,body,created_at FROM comments WHERE post_id=?",
		postID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	// load all comments in memory
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
			parent := allComments[*c.ParentID]
			parent.Replies = append(parent.Replies, c)
			allComments[*c.ParentID] = parent
		}
	}
	for i := range topLevel {
		topLevel[i] = allComments[topLevel[i].ID]
	}

	writeJSON(w, http.StatusOK, topLevel)
}

func votePost(w http.ResponseWriter, r *http.Request, postID int) {
	author, ok := r.Context().Value("username").(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req VoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Value != 1 && req.Value != -1 {
		writeError(w, http.StatusBadRequest, "value must be 1 or -1")
		return
	}

	_, err := db.Exec(
		"INSERT OR REPLACE INTO votes (post_id,username,value) VALUES (?,?,?)",
		postID, author, req.Value,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "voted"})
}
