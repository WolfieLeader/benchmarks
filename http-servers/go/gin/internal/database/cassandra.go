package database

import (
	"errors"
	"strings"
	"sync"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
)

type CassandraRepository struct {
	session       *gocql.Session
	contactPoints []string
	localDC       string
	keyspace      string
	once          sync.Once
	initErr       error
	mu            sync.Mutex
}

func NewCassandraRepository(contactPoints []string, localDC, keyspace string) *CassandraRepository {
	return &CassandraRepository{
		contactPoints: contactPoints,
		localDC:       localDC,
		keyspace:      keyspace,
	}
}

func (r *CassandraRepository) connect() error {
	r.once.Do(func() {
		cluster := gocql.NewCluster(r.contactPoints...)
		cluster.Keyspace = r.keyspace
		cluster.Consistency = gocql.Quorum
		if r.localDC != "" {
			cluster.PoolConfig.HostSelectionPolicy = gocql.DCAwareRoundRobinPolicy(r.localDC)
		}
		session, err := cluster.CreateSession()
		if err != nil {
			r.initErr = err
			return
		}
		r.session = session
	})
	return r.initErr
}

func (r *CassandraRepository) Create(data *CreateUser) (*User, error) {
	if err := r.connect(); err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	idStr := id.String()

	var query string
	var params []any
	if data.FavoriteNumber != nil {
		query = `INSERT INTO users (id, name, email, favorite_number) VALUES (?, ?, ?, ?)`
		params = []any{idStr, data.Name, data.Email, *data.FavoriteNumber}
	} else {
		query = `INSERT INTO users (id, name, email) VALUES (?, ?, ?)`
		params = []any{idStr, data.Name, data.Email}
	}

	if err := r.session.Query(query, params...).Exec(); err != nil {
		return nil, err
	}

	return BuildUser(idStr, data), nil
}

func (r *CassandraRepository) FindById(id string) (*User, error) {
	if err := r.connect(); err != nil {
		return nil, err
	}

	query := `SELECT id, name, email, favorite_number FROM users WHERE id = ?`
	var userId, name, email string
	var favoriteNumber *int

	if err := r.session.Query(query, id).Scan(&userId, &name, &email, &favoriteNumber); err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	user := &User{Id: userId, Name: name, Email: email, FavoriteNumber: favoriteNumber}
	return user, nil
}

func (r *CassandraRepository) Update(id string, data *UpdateUser) (*User, error) {
	if err := r.connect(); err != nil {
		return nil, err
	}

	existing, err := r.FindById(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	setClauses := []string{}
	params := []any{}

	if data.Name != nil {
		setClauses = append(setClauses, "name = ?")
		params = append(params, *data.Name)
		existing.Name = *data.Name
	}
	if data.Email != nil {
		setClauses = append(setClauses, "email = ?")
		params = append(params, *data.Email)
		existing.Email = *data.Email
	}
	if data.FavoriteNumber != nil {
		setClauses = append(setClauses, "favorite_number = ?")
		params = append(params, *data.FavoriteNumber)
		existing.FavoriteNumber = data.FavoriteNumber
	}

	if len(setClauses) == 0 {
		return existing, nil
	}

	params = append(params, id)
	var sb strings.Builder
	sb.WriteString("UPDATE users SET ")
	for i, clause := range setClauses {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(clause)
	}
	sb.WriteString(" WHERE id = ?")
	query := sb.String()

	if err := r.session.Query(query, params...).Exec(); err != nil {
		return nil, err
	}

	return existing, nil
}

func (r *CassandraRepository) Delete(id string) (bool, error) {
	if err := r.connect(); err != nil {
		return false, err
	}

	existing, err := r.FindById(id)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, nil
	}

	query := `DELETE FROM users WHERE id = ?`
	if err := r.session.Query(query, id).Exec(); err != nil {
		return false, err
	}

	return true, nil
}

func (r *CassandraRepository) DeleteAll() error {
	if err := r.connect(); err != nil {
		return err
	}

	return r.session.Query(`TRUNCATE users`).Exec()
}

func (r *CassandraRepository) HealthCheck() (bool, error) {
	if err := r.connect(); err != nil {
		return false, err
	}

	err := r.session.Query(`SELECT now() FROM system.local`).Exec()
	return err == nil, err
}

func (r *CassandraRepository) Disconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session != nil {
		r.session.Close()
		r.session = nil
	}
	return nil
}
