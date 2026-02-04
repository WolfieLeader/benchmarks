package config

import (
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Env struct {
	ENV                    string
	HOST                   string
	PORT                   uint16
	PostgresUrl            string
	MongoDbUrl             string
	MongoDbDatabase        string
	RedisUrl               string
	CassandraContactPoints []string
	CassandraLocalDc       string
	CassandraKeyspace      string
}

const (
	defaultEnv                    = "dev"
	defaultHost                   = "0.0.0.0"
	defaultPort                   = 5003
	defaultPostgresUrl            = "postgres://postgres:postgres@localhost:5432/benchmarks"
	defaultMongoDbUrl             = "mongodb://localhost:27017"
	defaultMongoDbDatabase        = "benchmarks"
	defaultRedisUrl               = "redis://localhost:6379"
	defaultCassandraContactPoints = "localhost"
	defaultCassandraLocalDc       = "datacenter1"
	defaultCassandraKeyspace      = "benchmarks"
)

func LoadEnv() *Env {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	env := &Env{
		ENV:                    defaultEnv,
		HOST:                   defaultHost,
		PORT:                   defaultPort,
		PostgresUrl:            defaultPostgresUrl,
		MongoDbUrl:             defaultMongoDbUrl,
		MongoDbDatabase:        defaultMongoDbDatabase,
		RedisUrl:               defaultRedisUrl,
		CassandraContactPoints: parseContactPoints(defaultCassandraContactPoints),
		CassandraLocalDc:       defaultCassandraLocalDc,
		CassandraKeyspace:      defaultCassandraKeyspace,
	}

	if e, ok := os.LookupEnv("ENV"); ok {
		if e != "dev" && e != "prod" {
			log.Fatalf("Invalid ENV: %s", e)
		}
		env.ENV = e
	}

	if host, ok := os.LookupEnv("HOST"); ok {
		if ip := net.ParseIP(host); ip != nil {
			env.HOST = ip.String()
		} else if host == "localhost" {
			env.HOST = "0.0.0.0"
		} else {
			log.Fatalf("Invalid HOST: %s", host)
		}
	}

	if n, err := strconv.ParseUint(os.Getenv("PORT"), 10, 16); err == nil {
		env.PORT = uint16(n)
	}

	if url, ok := os.LookupEnv("POSTGRES_URL"); ok && url != "" {
		env.PostgresUrl = url
	}
	if url, ok := os.LookupEnv("MONGODB_URL"); ok && url != "" {
		env.MongoDbUrl = url
	}
	if db, ok := os.LookupEnv("MONGODB_DB"); ok && db != "" {
		env.MongoDbDatabase = db
	}
	if url, ok := os.LookupEnv("REDIS_URL"); ok && url != "" {
		env.RedisUrl = url
	}
	if cp, ok := os.LookupEnv("CASSANDRA_CONTACT_POINTS"); ok && cp != "" {
		env.CassandraContactPoints = parseContactPoints(cp)
	}
	if dc, ok := os.LookupEnv("CASSANDRA_LOCAL_DATACENTER"); ok && dc != "" {
		env.CassandraLocalDc = dc
	}
	if ks, ok := os.LookupEnv("CASSANDRA_KEYSPACE"); ok && ks != "" {
		env.CassandraKeyspace = ks
	}

	return env
}

func parseContactPoints(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
