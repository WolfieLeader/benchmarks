package database

import (
	"sync"

	"gin-server/internal/config"
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

func InitializeConnections(env *config.Env) {
	var wg sync.WaitGroup
	for _, dbType := range DatabaseTypes {
		wg.Add(1)
		go func(dt DatabaseType) {
			defer wg.Done()
			repo := GetRepository(dt, env)
			if repo != nil {
				_, _ = repo.HealthCheck()
			}
		}(dbType)
	}
	wg.Wait()
}

type HealthStatus struct {
	Status    string            `json:"status"`
	Databases map[string]string `json:"databases"`
}

func GetAllHealthStatuses(env *config.Env) HealthStatus {
	result := HealthStatus{
		Status:    "healthy",
		Databases: make(map[string]string),
	}

	for _, dbType := range DatabaseTypes {
		repo := GetRepository(dbType, env)
		if repo == nil {
			result.Databases[string(dbType)] = "unhealthy"
			continue
		}

		healthy, _ := repo.HealthCheck()
		if healthy {
			result.Databases[string(dbType)] = "healthy"
		} else {
			result.Databases[string(dbType)] = "unhealthy"
		}
	}

	return result
}
