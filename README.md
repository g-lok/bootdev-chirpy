# boot.dev-chirpy

[boot.dev](https://boot.dev) http servers "chirpy" project

A simple backend REST API that allows users to post "chirps" (twitter style text tweets, basically).

## Why

Example of using Go to build a basic RESTful API server with a PostgreSQL backend, using `goose`, `sqlc`, and other helpful tools and libs. Also includes examples of authentication/authorization using JWTs and Refresh Tokens.

## Install {localhost}

1. Clone the repo
1. Create a `PostgreSQL` server with a `chirpy` db. Get the connection URL. i.e. `postgres://[user]:[pass]@localhost:5432/chirpy?sslmode=disable`
1. Create a `.env` file in the project root with the following ENV variables:

   ```bash
   DB_URL="[see above]"
   PLATFORM=dev
   SECRET="[openssl rand -base64 64]"
   POLKA_KEY="f271c81ff7084ee5b99a5091b42d486e" # a pretend 3rd party webhook api key
   ```

1. Run `go mod download`
1. Launch the server with `go run .`

## API

- `POST /admin/reset`: resets the db clean
- `GET /api/healthz`: health check
- `POST /api/users`: Create a new user.

  ```json
  {
    "email": "...",
    "password": "..."
  }
  ```

- `PUT /api/users`: Update user/pass. Requires `Authorization: Bearer [token]` header.

  ```json
  {
    "email": "..."
    "password": "..."
  }
  ```

- `POST /api/login`: Login and receive JWT token.

  ```json
  {
    "email": "...",
    "password": "..."
  }
  ```

- `POST /api/refresh`: Checks refresh token. Requires `Authorization: Bearer [token]` header.

  ```json
  {
    "token": "..."
  }
  ```

- `POST /api/revoke`: Revokes a refresh token. Requires `Authorization: Bearer [token]` header.
- `POST /api/chirps`: Create a new chirp. Requires `Authorization: Bearer [token]` header.

  ```json
  {
    "body": "..."
  }
  ```

- `GET /api/chirps`: Gets a list of chirps. Query parameters:
  - `author_id`: filter by user_id.
  - `sort`: Sort results by either `asc` or `desc` created_at.
- `GET /api/chirps/{chirpID}`: Get a specific chirp by ID
- `DELETE /api/chirps/{chirpID}`: Delete chirp by ID. Requires authorization as the chirp author.
- `POST /api/polka/webhooks`: Mock webhook for fictional 3rd-party "Polka" provider.

## Author

G <g@devnull.rip>
