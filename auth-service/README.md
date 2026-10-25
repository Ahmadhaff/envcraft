# auth-service

Clean Architecture service (`github.com/ahmadhaffoudhi/auth-service`).

Run the tests:

    go test ./...

Start the server:

    go run ./cmd/server

Routes:

- `GET /health`
- `POST /users` with JSON fields `id`, `email`, and `name`
- `GET /users/{id}`
