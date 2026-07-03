-- name: CreateUser :one
INSERT INTO users (id, name, email, favorite_number)
VALUES ($1, $2, $3, $4)
RETURNING id, name, email, favorite_number;

-- name: GetUserById :one
SELECT id, name, email, favorite_number FROM users WHERE id = $1;

-- name: UpdateUser :one
UPDATE users SET
    name = COALESCE(sqlc.narg('name'), name),
    email = COALESCE(sqlc.narg('email'), email),
    favorite_number = COALESCE(sqlc.narg('favorite_number'), favorite_number)
WHERE id = $1
RETURNING id, name, email, favorite_number;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;

-- name: DeleteAllUsers :exec
DELETE FROM users;

-- name: HealthCheck :exec
SELECT 1;
