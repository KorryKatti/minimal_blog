package main

import (
	"context" // for request and server contexts
	"encoding/json" // for json stuff
	// "errors" // obv
	"fmt" // for printing
	"io"
	"net" // for networok addresses
	"net/http" // for http server
	"sync" // sync access to shared memory // idk what that means yet
)

const keyServerAddr = "serverAddr"

type User struct {
	Username string `json:"username"` // name type tag ( tag is like a label for the field so that the json encoder knows what to name the field when encoding)
	Password string `json:"password"` // same thing here	
}

// memory map
var ( // var is used for global variables
	users = make(map[string]string) // map to store users , format is <username,password>
	usersMu sync.Mutex // mutex to sync access to users map , makes it not have race conditions
)

func main() {
	fmt.Println("Hello, minimal blog")

	mux := http.NewServeMux()
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/hello", getHello)
	mux.HandleFunc("/signup", signupHandler)
	mux.HandleFunc("/signin", signinHandler)

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