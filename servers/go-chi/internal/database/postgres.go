package database

import (
	"context"
	"errors"
	"sync"

	"chi-server/internal/database/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	url     string
	once    sync.Once
	initErr error
	mu      sync.Mutex
}

func NewPostgresRepository(connectionString string) *PostgresRepository {
	return &PostgresRepository{url: connectionString}
}

func (r *PostgresRepository) connect(ctx context.Context) error {
	r.once.Do(func() {
		config, err := pgxpool.ParseConfig(r.url)
		if err != nil {
			r.initErr = err
			return
		}
		config.MaxConns = 50
		config.MinConns = 10
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			r.initErr = err
			return
		}
		r.pool = pool
		r.queries = sqlc.New(pool)
	})
	return r.initErr
}

func toUser(u sqlc.User) *User {
	var favoriteNumber *int
	if u.FavoriteNumber != nil {
		n := int(*u.FavoriteNumber)
		favoriteNumber = &n
	}
	return &User{
		Id:             uuidToString(u.ID),
		Name:           u.Name,
		Email:          u.Email,
		FavoriteNumber: favoriteNumber,
	}
}

func uuidToString(u pgtype.UUID) string {
	return uuid.UUID(u.Bytes).String()
}

func stringToUUID(s string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func intToInt32Ptr(n *int) *int32 {
	if n == nil {
		return nil
	}
	v := int32(*n) //nolint:gosec // FavoriteNumber is validated to be 0-100
	return &v
}

func (r *PostgresRepository) Create(ctx context.Context, data *CreateUser) (*User, error) {
	if err := r.connect(ctx); err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	user, err := r.queries.CreateUser(ctx, sqlc.CreateUserParams{
		ID:             pgtype.UUID{Bytes: id, Valid: true},
		Name:           data.Name,
		Email:          data.Email,
		FavoriteNumber: intToInt32Ptr(data.FavoriteNumber),
	})
	if err != nil {
		return nil, err
	}
	return toUser(user), nil
}

func (r *PostgresRepository) FindById(ctx context.Context, id string) (*User, error) {
	if err := r.connect(ctx); err != nil {
		return nil, err
	}

	pgId, err := stringToUUID(id)
	if err != nil {
		return nil, nil //nolint:nilerr // invalid UUID means user not found
	}

	user, err := r.queries.GetUserById(ctx, pgId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toUser(user), nil
}

func (r *PostgresRepository) Update(ctx context.Context, id string, data *UpdateUser) (*User, error) {
	if err := r.connect(ctx); err != nil {
		return nil, err
	}

	if data.Name == nil && data.Email == nil && data.FavoriteNumber == nil {
		return r.FindById(ctx, id)
	}

	pgId, err := stringToUUID(id)
	if err != nil {
		return nil, nil //nolint:nilerr // invalid UUID means user not found
	}

	user, err := r.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
		ID:             pgId,
		Name:           data.Name,
		Email:          data.Email,
		FavoriteNumber: intToInt32Ptr(data.FavoriteNumber),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toUser(user), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) (bool, error) {
	if err := r.connect(ctx); err != nil {
		return false, err
	}

	pgId, err := stringToUUID(id)
	if err != nil {
		return false, nil //nolint:nilerr // invalid UUID means user not found
	}

	rows, err := r.queries.DeleteUser(ctx, pgId)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *PostgresRepository) DeleteAll(ctx context.Context) error {
	if err := r.connect(ctx); err != nil {
		return err
	}

	return r.queries.DeleteAllUsers(ctx)
}

func (r *PostgresRepository) HealthCheck(ctx context.Context) (bool, error) {
	if err := r.connect(ctx); err != nil {
		return false, nil //nolint:nilerr // connection failure means unhealthy, not error
	}

	err := r.queries.HealthCheck(ctx)
	return err == nil, nil
}

func (r *PostgresRepository) Disconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pool != nil {
		r.pool.Close()
		r.pool = nil
		r.queries = nil
	}
	return nil
}
