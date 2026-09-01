-- name: GetUserByEmail :one
SELECT * from users
WHERE email = $1;
