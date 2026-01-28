CREATE KEYSPACE IF NOT EXISTS benchmarks
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE benchmarks;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    name TEXT,
    email TEXT,
    favorite_number INT
);
