-- name: UpdateUserRed :exec
UPDATE users
  SET is_chirpy_red = TRUE,
      updated_at = NOW()
WHERE id = $1;
