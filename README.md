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
- [ ] JSON responses (not plain text)
- [ ] Proper Content-Type headers
- [ ] CORS headers for custom frontends
- [ ] Consistent error response format
- [ ] API versioning (`/api/v1/...`)

### Security
- [ ] Rate limiting
- [ ] Input validation / sanitization
