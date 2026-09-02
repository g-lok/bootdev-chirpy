-- name: UserExistsByID :one
SELECT EXISTS(
    SELECT 1 FROM users WHERE id = @id
);

