package repository

import (
	"context"
	"database/sql"
	"time"

	"flowscale/internal/models"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) CreateUser(ctx context.Context, user *models.User) error {
	user.CreatedAt = time.Now()
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO users (id, username, password_hash, created_at) VALUES ($1, $2, $3, $4)",
		user.ID, user.Username, user.PasswordHash, user.CreatedAt,
	)
	return err
}

func (r *UserRepo) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := r.db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, created_at FROM users WHERE username = $1",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Return nil if not found
		}
		return nil, err
	}
	return &user, nil
}
