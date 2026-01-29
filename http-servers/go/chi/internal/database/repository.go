package database

import (
	"sync"

	"chi-server/internal/config"
)

type UserRepository interface {
	Create(data *CreateUser) (*User, error)
	FindById(id string) (*User, error)
	Update(id string, data *UpdateUser) (*User, error)
	Delete(id string) (bool, error)
	DeleteAll() error
	HealthCheck() (bool, error)
	Disconnect() error
}

type DatabaseType string

const (
	DatabasePostgres  DatabaseType = "postgres"
	DatabaseMongoDB   DatabaseType = "mongodb"
	DatabaseRedis     DatabaseType = "redis"
	DatabaseCassandra DatabaseType = "cassandra"
)

var databaseTypes = []DatabaseType{
	DatabasePostgres,
	DatabaseMongoDB,
	DatabaseRedis,
	DatabaseCassandra,
}

func IsDatabaseType(value string) bool {
	for _, dt := range databaseTypes {
		if string(dt) == value {
			return true
		}
	}
	return false
}

var (
	repositories = make(map[DatabaseType]UserRepository)
	mu           sync.RWMutex
)

func GetRepository(database DatabaseType, env *config.Env) UserRepository {
	mu.RLock()
	repo, exists := repositories[database]
	mu.RUnlock()
	if exists {
		return repo
	}

	mu.Lock()
	defer mu.Unlock()

	if repo, exists = repositories[database]; exists {
		return repo
	}

	switch database {
	case DatabasePostgres:
		repo = NewPostgresRepository(env.PostgresURL)
	case DatabaseMongoDB:
		repo = NewMongoRepository(env.MongoDBURL, env.MongoDBDatabase)
	case DatabaseRedis:
		repo = NewRedisRepository(env.RedisURL)
	case DatabaseCassandra:
		repo = NewCassandraRepository(env.CassandraContactPoints, env.CassandraLocalDC, env.CassandraKeyspace)
	default:
		return nil
	}

	repositories[database] = repo
	return repo
}

func ResolveRepository(database string, env *config.Env) UserRepository {
	if !IsDatabaseType(database) {
		return nil
	}
	return GetRepository(DatabaseType(database), env)
}
