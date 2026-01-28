-- databases/cassandra/init.cql
CREATE KEYSPACE IF NOT EXISTS bench
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE bench;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    name TEXT,
    email TEXT
);
