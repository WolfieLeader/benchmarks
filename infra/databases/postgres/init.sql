-- databases/postgres/init.sql
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    favorite_number INTEGER
);

CREATE INDEX idx_users_email ON users(email);
