# minimal_blog

A minimal blog engine you run from your terminal. Write posts from the CLI, serve them via a lightweight API, and bring your own frontend.

## What it does

- Blog from your terminal — no web UI required
- Deploy the backend once (Render, AWS, etc.), send posts from your terminal from anywhere
- Your frontend (or anyone's) reads from the same API — bring your own
- Stays minimal: single binary, SQLite, no framework

## TODO

### Persistence
- [x] Replace in-memory maps with SQLite
- [x] Auto-create DB + tables on startup

### Blog system
- [x] Blog post model (id, title, body, author, created_at)
- [x] Create post endpoint `POST /api/posts`
- [x] List posts endpoint `GET /api/posts`
- [x] Get single post endpoint `GET /api/posts/:id`
- [x] Delete post endpoint `DELETE /api/posts/:id`

### Auth
- [x] Hash passwords with bcrypt
- [x] Token-based auth (JWT or simple tokens)
- [x] Auth middleware for protected routes
- [x] Terminal app sends auth headers

### Engagement
- [x] Comments — create and list per post
- [x] Replies to comments
- [x] Upvotes / downvotes on posts
- [x] Vote counts in post responses

### API polish
- [x] JSON responses (not plain text)
- [x] CORS headers for custom frontends
- [x] Consistent error response format

### Security
- [x] Rate limiting
- [x] Input validation / sanitization

## Self-hosting

Build and run the server:

```bash
cd server
go build -o minimal_blog .
./minimal_blog
```

The server starts on `http://localhost:8080` by default. It creates a SQLite database file (`blog.db`) on first run.

You can set a port and token secret via environment variables:

```bash
PORT=3000 TOKEN_SECRET=your-secret ./minimal_blog
```

## Free hosting options

You can deploy the backend to any provider that supports Go.

For most of these you'll need a `Dockerfile`. Minimal example:

```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o server .

FROM alpine:3.19
COPY --from=build /app/server /server
EXPOSE 8080
CMD ["/server"]
```

## Using the CLI client

The client lets you blog from your terminal and read posts.

### Setup

```bash
cd client
go build -o blogclient .
```

### Authenticate

```bash
./blogclient register --server https://your-server.fly.dev --username you --password secret
```

Or log in if you already have an account:

```bash
./blogclient login --server https://your-server.fly.dev --username you --password secret
```

### Create a post

```bash
./blogclient create --title "My First Post" --body "Hello from the terminal!"
```

Pipe from stdin:

```bash
echo "This is my post content" | ./blogclient create --title "Piped Post"
```

Edit with your `$EDITOR`:

```bash
./blogclient create --editor
```

### Read posts

```bash
./blogclient list
./blogclient read --id 3
```

### Delete a post

```bash
./blogclient delete --id 3
```

## Website

I may or may not build a web frontend for this. The API is open — feel free to bring your own.
