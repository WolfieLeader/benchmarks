package database

import (
	"context"
	"sync"

	"fiber-server/internal/config"
)

type UserRepository interface {
	Create(ctx context.Context, data *CreateUser) (*User, error)
	FindById(ctx context.Context, id string) (*User, error)
	Update(ctx context.Context, id string, data *UpdateUser) (*User, error)
	Delete(ctx context.Context, id string) (bool, error)
	DeleteAll(ctx context.Context) error
	HealthCheck(ctx context.Context) (bool, error)
	Disconnect() error
}

type DatabaseType string

const (
	DatabasePostgres  DatabaseType = "postgres"
	DatabaseMongoDB   DatabaseType = "mongodb"
	DatabaseRedis     DatabaseType = "redis"
	DatabaseCassandra DatabaseType = "cassandra"
)

var DatabaseTypes = []DatabaseType{
	DatabasePostgres,
	DatabaseMongoDB,
	DatabaseRedis,
	DatabaseCassandra,
}

func IsDatabaseType(value string) bool {
	for _, dt := range DatabaseTypes {
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
		repo = NewPostgresRepository(env.PostgresUrl)
	case DatabaseMongoDB:
		repo = NewMongoRepository(env.MongoDbUrl, env.MongoDbDatabase)
	case DatabaseRedis:
		repo = NewRedisRepository(env.RedisUrl)
	case DatabaseCassandra:
		repo = NewCassandraRepository(env.CassandraContactPoints, env.CassandraLocalDc, env.CassandraKeyspace)
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

func InitializeConnections(env *config.Env) {
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, dbType := range DatabaseTypes {
		wg.Add(1)
		go func(dt DatabaseType) {
			defer wg.Done()
			repo := GetRepository(dt, env)
			if repo != nil {
				_, _ = repo.HealthCheck(ctx)
			}
		}(dbType)
	}
	wg.Wait()
}
