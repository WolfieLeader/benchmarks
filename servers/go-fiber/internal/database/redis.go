package database

import (
	"context"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisRepository struct {
	client  *redis.Client
	url     string
	prefix  string
	once    sync.Once
	initErr error
	mu      sync.Mutex
}

func NewRedisRepository(connectionString string) *RedisRepository {
	return &RedisRepository{url: connectionString, prefix: "user:"}
}

func (r *RedisRepository) connect() error {
	r.once.Do(func() {
		opt, err := redis.ParseURL(r.url)
		if err != nil {
			r.initErr = err
			return
		}
		r.client = redis.NewClient(opt)
	})
	return r.initErr
}

func (r *RedisRepository) key(id string) string {
	return r.prefix + id
}

func (r *RedisRepository) Create(ctx context.Context, data *CreateUser) (*User, error) {
	if err := r.connect(); err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	idStr := id.String()

	fields := map[string]any{
		"name":  data.Name,
		"email": data.Email,
	}
	if data.FavoriteNumber != nil {
		fields["favoriteNumber"] = strconv.Itoa(*data.FavoriteNumber)
	}

	if err := r.client.HSet(ctx, r.key(idStr), fields).Err(); err != nil {
		return nil, err
	}

	return BuildUser(idStr, data), nil
}

func (r *RedisRepository) FindById(ctx context.Context, id string) (*User, error) {
	if err := r.connect(); err != nil {
		return nil, err
	}

	key := r.key(id)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, nil
	}

	result, err := r.client.HMGet(ctx, key, "name", "email", "favoriteNumber").Result()
	if err != nil {
		return nil, err
	}

	if len(result) < 2 || result[0] == nil || result[1] == nil {
		return nil, nil
	}

	name, ok := result[0].(string)
	if !ok {
		return nil, nil
	}
	email, ok := result[1].(string)
	if !ok {
		return nil, nil
	}

	user := &User{Id: id, Name: name, Email: email}
	if len(result) > 2 && result[2] != nil {
		if favStr, ok := result[2].(string); ok {
			if fav, err := strconv.Atoi(favStr); err == nil {
				user.FavoriteNumber = &fav
			}
		}
	}

	return user, nil
}

func (r *RedisRepository) Update(ctx context.Context, id string, data *UpdateUser) (*User, error) {
	if err := r.connect(); err != nil {
		return nil, err
	}

	key := r.key(id)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, nil
	}

	fields := make(map[string]any)
	if data.Name != nil {
		fields["name"] = *data.Name
	}
	if data.Email != nil {
		fields["email"] = *data.Email
	}
	if data.FavoriteNumber != nil {
		fields["favoriteNumber"] = strconv.Itoa(*data.FavoriteNumber)
	}

	if len(fields) > 0 {
		if err := r.client.HSet(ctx, key, fields).Err(); err != nil {
			return nil, err
		}
	}

	return r.FindById(ctx, id)
}

func (r *RedisRepository) Delete(ctx context.Context, id string) (bool, error) {
	if err := r.connect(); err != nil {
		return false, err
	}

	deleted, err := r.client.Del(ctx, r.key(id)).Result()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (r *RedisRepository) DeleteAll(ctx context.Context) error {
	if err := r.connect(); err != nil {
		return err
	}

	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, r.prefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

func (r *RedisRepository) HealthCheck(ctx context.Context) (bool, error) {
	if err := r.connect(); err != nil {
		return false, err
	}

	_, err := r.client.Ping(ctx).Result()
	return err == nil, err
}

func (r *RedisRepository) Disconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		err := r.client.Close()
		r.client = nil
		return err
	}
	return nil
}
