package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	serverURL string
	token     string
	username  string
	reader    = bufio.NewReader(os.Stdin)
	client    = &http.Client{Timeout: 10 * time.Second}
)

const (
	bold  = "\033[1m"
	dim   = "\033[2m"
	red   = "\033[31m"
	green = "\033[32m"
	yellow = "\033[33m"
	cyan  = "\033[36m"
	reset = "\033[0m"
)

type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Printf("Usage: %s <server-url>\n\n", os.Args[0])
		fmt.Printf("Example: %s %shttp://localhost:3456%s\n\n", os.Args[0], cyan, reset)
		fmt.Printf("%s%sCommands:%s\n", bold, yellow, reset)
		fmt.Printf("  %ssignup%s  <user> <pass>           Create account\n", cyan, reset)
		fmt.Printf("  %ssignin%s  <user> <pass>            Login & get token\n", cyan, reset)
		fmt.Printf("  %swhoami%s                           Show current user\n", cyan, reset)
		fmt.Println()
		fmt.Printf("  %slist%s                             List all posts\n", cyan, reset)
		fmt.Printf("  %sget%s    <id>                      View post + comments\n", cyan, reset)
		fmt.Printf("  %screate%s                           Create a post (prompts for title & body)\n", cyan, reset)
		fmt.Printf("  %sdelete%s <id>                      Delete a post\n", cyan, reset)
		fmt.Println()
		fmt.Printf("  %scomment%s <post_id>                Add comment (prompts for body)\n", cyan, reset)
		fmt.Printf("  %scomment%s <post_id>                Reply to comment (prompts for body & parent)\n", cyan, reset)
		fmt.Printf("  %svote%s   <post_id> <1|-1>          Upvote/downvote\n", cyan, reset)
		fmt.Println()
		fmt.Printf("  %shello%s                            Test connection\n", cyan, reset)
		fmt.Printf("  %squit%s                             Exit\n", cyan, reset)
		fmt.Println()
		fmt.Printf("%sTip:%s args are optional — the client will prompt for them interactively.\n", dim, reset)
		if len(os.Args) < 2 {
			os.Exit(1)
		}
		return
	}
	serverURL = strings.TrimRight(os.Args[1], "/")

	printBanner()
	printHelp()

	for {
		fmt.Printf("%s%sblog>%s ", bold, green, reset)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd, args := parts[0], parts[1:]

		switch cmd {
		case "help", "h", "?":
			printHelp()
		case "signup", "su":
			cmdSignup(args)
		case "signin", "si":
			cmdSignin(args)
		case "whoami":
			cmdWhoami()
		case "list", "ls":
			cmdListPosts()
		case "get", "show":
			cmdGetPost(args)
		case "create", "new", "post":
			cmdCreatePost(args)
		case "delete", "rm":
			cmdDeletePost(args)
		case "comment", "reply":
			cmdComment(args)
		case "vote":
			cmdVote(args)
		case "hello":
			cmdHello()
		case "quit", "exit", "q":
			fmt.Printf("%sGoodbye!%s\n", yellow, reset)
			return
		default:
			fmt.Printf("%sUnknown command:%s %s (type %shelp%s for commands)\n", red, reset, cmd, cyan, reset)
		}
	}
}

func printBanner() {
	fmt.Println()
	fmt.Printf("%s%s╔══════════════════════════════════════╗%s\n", bold, cyan, reset)
	fmt.Printf("%s%s║       Minimal Blog Client            ║%s\n", bold, cyan, reset)
	fmt.Printf("%s%s╚══════════════════════════════════════╝%s\n", bold, cyan, reset)
	fmt.Printf("%sConnected to:%s %s%s%s\n\n", dim, reset, cyan, serverURL, reset)
}

func printHelp() {
	fmt.Printf("%s%sCommands:%s\n", bold, yellow, reset)
	fmt.Printf("  %ssignup%s <user> <pass>           Create account\n", cyan, reset)
	fmt.Printf("  %ssignin%s <user> <pass>            Login & get token\n", cyan, reset)
	fmt.Printf("  %swhoami%s                          Show current user\n", cyan, reset)
	fmt.Println()
	fmt.Printf("  %slist%s                            List all posts\n", cyan, reset)
	fmt.Printf("  %sget%s <id>                        View post + comments\n", cyan, reset)
	fmt.Printf("  %screate%s                           Create a post (prompts for title & body)\n", cyan, reset)
	fmt.Printf("  %sdelete%s <id>                     Delete a post\n", cyan, reset)
	fmt.Println()
	fmt.Printf("  %scomment%s <post_id>                Add comment (prompts for body)\n", cyan, reset)
	fmt.Printf("  %scomment%s <post_id>                Reply to comment (prompts for body & parent)\n", cyan, reset)
	fmt.Printf("  %svote%s <post_id> <1|-1>           Upvote/downvote\n", cyan, reset)
	fmt.Println()
	fmt.Printf("  %shello%s                           Test connection\n", cyan, reset)
	fmt.Printf("  %squit%s                            Exit\n", cyan, reset)
	fmt.Println()
}

func requireAuth() bool {
	if token == "" {
		fmt.Printf("%sNot signed in. Run %ssignin%s first.%s\n", red, bold, reset, reset)
		return false
	}
	return true
}

// ---- HTTP helpers ----

func doRequest(method, path string, body interface{}) (*APIResponse, int) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			fmt.Printf("%sError:%s %s\n", red, reset, err)
			return nil, 0
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, serverURL+path, reqBody)
	if err != nil {
		fmt.Printf("%sError:%s %s\n", red, reset, err)
		return nil, 0
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%sError:%s %s\n", red, reset, err)
		return nil, 0
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var apiResp APIResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			fmt.Printf("%sError:%s invalid response: %s\n", red, reset, string(respBody))
			return nil, 0
		}
	}
	return &apiResp, resp.StatusCode
}

func printError(msg string) {
	fmt.Printf("%s%s Error: %s%s\n", bold, red, msg, reset)
}

func printSuccess(msg string) {
	fmt.Printf("%s%s %s%s\n", bold, green, msg, reset)
}

// ---- Commands ----

func cmdSignup(args []string) {
	user, pass := "", ""
	if len(args) >= 2 {
		user, pass = args[0], args[1]
	} else {
		user = prompt("Username: ")
		pass = promptPassword("Password: ")
	}
	if user == "" || pass == "" {
		printError("username and password required")
		return
	}

	resp, status := doRequest("POST", "/signup", map[string]string{"username": user, "password": pass})
	if resp == nil {
		return
	}
	if status == http.StatusCreated {
		printSuccess("Account created! Now run signin.")
	} else {
		printError(resp.Error)
	}
}

func cmdSignin(args []string) {
	user, pass := "", ""
	if len(args) >= 2 {
		user, pass = args[0], args[1]
	} else {
		user = prompt("Username: ")
		pass = promptPassword("Password: ")
	}
	if user == "" || pass == "" {
		printError("username and password required")
		return
	}

	resp, status := doRequest("POST", "/signin", map[string]string{"username": user, "password": pass})
	if resp == nil {
		return
	}
	if status == http.StatusOK {
		data, _ := json.Marshal(resp.Data)
		var result struct {
			Token string `json:"token"`
		}
		json.Unmarshal(data, &result)
		token = result.Token
		username = user
		printSuccess(fmt.Sprintf("Signed in as %s%s%s!", bold, username, reset))
	} else {
		printError(resp.Error)
	}
}

func cmdWhoami() {
	if token == "" {
		fmt.Printf("%sNot signed in%s\n", dim, reset)
	} else {
		fmt.Printf("%sLogged in as:%s %s%s%s\n", dim, reset, bold, username, reset)
	}
}

func cmdHello() {
	resp, _ := doRequest("GET", "/hello", nil)
	if resp == nil {
		return
	}
	if data, ok := resp.Data.(string); ok {
		fmt.Println(data)
	} else {
		fmt.Printf("%s%v%s\n", cyan, resp.Data, reset)
	}
}

func cmdListPosts() {
	resp, status := doRequest("GET", "/posts", nil)
	if resp == nil {
		return
	}
	if status != http.StatusOK {
		printError(resp.Error)
		return
	}

	data, _ := json.Marshal(resp.Data)
	var posts []struct {
		ID        int       `json:"id"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		Author    string    `json:"author"`
		CreatedAt time.Time `json:"created_at"`
		Upvotes   int       `json:"upvotes"`
		Downvotes int       `json:"downvotes"`
		Score     int       `json:"score"`
	}
	json.Unmarshal(data, &posts)

	if len(posts) == 0 {
		fmt.Printf("%sNo posts yet. Be the first to %screate%s one!%s\n", dim, cyan, dim, reset)
		return
	}

	fmt.Printf("%s%s%-4s %-30s %-12s  SCORE%s\n", bold, reset, "ID", "TITLE", "BY", reset)
	fmt.Printf("%s%s%s\n", dim, strings.Repeat("─", 60), reset)
	for _, p := range posts {
		title := p.Title
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		scoreColor := green
		if p.Score < 0 {
			scoreColor = red
		} else if p.Score == 0 {
			scoreColor = dim
		}
		fmt.Printf("  %s%-3d%s %s%-30s%s %s%-12s%s %s%+d%s\n",
			cyan, p.ID, reset,
			bold, title, reset,
			dim, p.Author, reset,
			scoreColor, p.Score, reset,
		)
	}
	fmt.Printf("\n%sType %sget <id>%s to view details.%s\n", dim, cyan, dim, reset)
}

func cmdGetPost(args []string) {
	if len(args) < 1 {
		id := prompt("Post ID: ")
		if id == "" {
			printError("post ID required")
			return
		}
		args = []string{id}
	}

	resp, status := doRequest("GET", "/posts/"+args[0], nil)
	if resp == nil {
		return
	}
	if status != http.StatusOK {
		printError(resp.Error)
		return
	}

	data, _ := json.Marshal(resp.Data)
	var post struct {
		ID        int       `json:"id"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		Author    string    `json:"author"`
		CreatedAt time.Time `json:"created_at"`
		Upvotes   int       `json:"upvotes"`
		Downvotes int       `json:"downvotes"`
		Score     int       `json:"score"`
	}
	json.Unmarshal(data, &post)

	fmt.Printf("\n%s%s#%d %s%s\n", bold, cyan, post.ID, post.Title, reset)
	fmt.Printf("%sBy:%s %s  %s│%s  %s%s%s  %s│%s  %s%s\n",
		dim, reset, post.Author,
		dim, reset,
		green, fmt.Sprintf("▲%d", post.Upvotes), reset,
		dim, reset,
		red, fmt.Sprintf("▼%d", post.Downvotes), reset,
	)
	fmt.Printf("%sPosted:%s %s\n\n", dim, reset, post.CreatedAt.Format("2006-01-02 15:04"))
	fmt.Println(post.Body)

	fmt.Printf("\n%s%s── Comments ──%s\n", dim, strings.Repeat("─", 40), reset)

	// Fetch comments
	cResp, _ := doRequest("GET", "/posts/"+args[0]+"/comments", nil)
	if cResp == nil {
		return
	}
	cData, _ := json.Marshal(cResp.Data)
	var comments []struct {
		ID       int    `json:"id"`
		Author   string `json:"author"`
		Body     string `json:"body"`
		ParentID *int   `json:"parent_id,omitempty"`
		Replies  []struct {
			ID     int    `json:"id"`
			Author string `json:"author"`
			Body   string `json:"body"`
		} `json:"replies,omitempty"`
	}
	json.Unmarshal(cData, &comments)

	if len(comments) == 0 {
		fmt.Printf("%sNo comments yet.%s\n", dim, reset)
	} else {
		for _, c := range comments {
			printComment(c.ID, c.Author, c.Body, 0)
			for _, r := range c.Replies {
				printComment(r.ID, r.Author, r.Body, 1)
			}
		}
	}
	fmt.Println()
}

func printComment(id int, author, body string, depth int) {
	indent := strings.Repeat("  ", depth)
	prefix := "├─"
	if depth > 0 {
		prefix = "└─"
	}
	fmt.Printf("%s%s %s%s%s: %s\n", indent, prefix, bold, author, reset, body)
}

func cmdCreatePost(args []string) {
	if !requireAuth() {
		return
	}

	title := prompt("Title: ")
	body := prompt("Body: ")
	if title == "" || body == "" {
		printError("title and body required")
		return
	}

	resp, status := doRequest("POST", "/api/posts", map[string]string{"title": title, "body": body})
	if resp == nil {
		return
	}
	if status == http.StatusCreated {
		data, _ := json.Marshal(resp.Data)
		var p struct {
			ID int `json:"id"`
		}
		json.Unmarshal(data, &p)
		printSuccess(fmt.Sprintf("Post created! ID: %s#%d%s", bold, p.ID, reset))
	} else {
		printError(resp.Error)
	}
}

func cmdDeletePost(args []string) {
	if !requireAuth() {
		return
	}

	id := ""
	if len(args) >= 1 {
		id = args[0]
	} else {
		id = prompt("Post ID: ")
	}
	if id == "" {
		printError("post ID required")
		return
	}

	confirm := prompt(fmt.Sprintf("Delete post #%s? (y/n): ", id))
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Printf("%sCancelled.%s\n", dim, reset)
		return
	}

	resp, status := doRequest("DELETE", "/api/posts/"+id, nil)
	if resp == nil {
		return
	}
	if status == http.StatusNoContent || status == http.StatusOK {
		printSuccess("Post deleted.")
	} else {
		printError(resp.Error)
	}
}

func cmdComment(args []string) {
	if !requireAuth() {
		return
	}

	postID, body, parentID := "", "", ""
	if len(args) >= 1 {
		postID = args[0]
	} else {
		postID = prompt("Post ID: ")
	}
	body = prompt("Comment: ")
	if reply := prompt("Reply to comment ID (blank for top-level): "); reply != "" {
		parentID = reply
	}

	if postID == "" || body == "" {
		printError("post ID and comment body required")
		return
	}

	payload := map[string]interface{}{"body": body}
	if parentID != "" {
		pid, _ := strconv.Atoi(parentID)
		payload["parent_id"] = pid
	}

	resp, status := doRequest("POST", "/api/posts/"+postID+"/comments", payload)
	if resp == nil {
		return
	}
	if status == http.StatusCreated {
		printSuccess("Comment posted!")
	} else {
		printError(resp.Error)
	}
}

func cmdVote(args []string) {
	if !requireAuth() {
		return
	}

	postID, value := "", ""
	if len(args) >= 2 {
		postID, value = args[0], args[1]
	} else {
		postID = prompt("Post ID: ")
		value = prompt("Vote (1=up, -1=down): ")
	}

	if postID == "" || value == "" {
		printError("post ID and vote value required")
		return
	}

	v, err := strconv.Atoi(value)
	if err != nil || (v != 1 && v != -1) {
		printError("vote must be 1 or -1")
		return
	}

	resp, status := doRequest("POST", "/api/posts/"+postID+"/vote", map[string]int{"value": v})
	if resp == nil {
		return
	}
	if status == http.StatusOK {
		if v == 1 {
			printSuccess("Upvoted!")
		} else {
			printSuccess("Downvoted!")
		}
	} else {
		printError(resp.Error)
	}
}

// ---- Prompt helpers ----

func prompt(label string) string {
	fmt.Printf("%s%s%s", dim, label, reset)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func promptPassword(label string) string {
	fmt.Printf("%s%s%s", dim, label, reset)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}
