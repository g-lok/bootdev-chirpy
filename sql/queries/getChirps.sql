-- name: GetChirps :many
SELECT * FROM chirps
ORDER BY chirps.created_at ASC;
