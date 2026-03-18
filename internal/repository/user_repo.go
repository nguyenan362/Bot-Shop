package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nguyenan362/bot-shop-go/internal/models"
	"github.com/shopspring/decimal"
)

// UserRepo handles user database operations.
type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

// Upsert creates or updates a user on /start.
func (r *UserRepo) Upsert(ctx context.Context, teleID int64, username string, isAdmin bool) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (tele_id, username, is_admin)
		VALUES ($1, $2, $3)
		ON CONFLICT (tele_id) DO UPDATE
		SET username = EXCLUDED.username,
		    is_admin = users.is_admin OR EXCLUDED.is_admin
	`, teleID, username, isAdmin)
	return err
}

// GetByID returns a user by Telegram ID.
func (r *UserRepo) GetByID(ctx context.Context, teleID int64) (*models.User, error) {
	u := &models.User{}
	err := r.pool.QueryRow(ctx, `
		SELECT tele_id, username, balance_usdt, language, timezone, join_date, is_admin, is_banned
		FROM users WHERE tele_id = $1
	`, teleID).Scan(&u.TeleID, &u.Username, &u.BalanceUSDT, &u.Language, &u.Timezone, &u.JoinDate, &u.IsAdmin, &u.IsBanned)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// UpdateLanguage sets user language preference.
func (r *UserRepo) UpdateLanguage(ctx context.Context, teleID int64, lang string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET language = $2 WHERE tele_id = $1`, teleID, lang)
	return err
}

// AddBalance adds USDT to user balance.
func (r *UserRepo) AddBalance(ctx context.Context, teleID int64, amount decimal.Decimal) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET balance_usdt = balance_usdt + $2 WHERE tele_id = $1
	`, teleID, amount)
	return err
}

// DeductBalance subtracts USDT from user balance. Returns error if insufficient.
func (r *UserRepo) DeductBalance(ctx context.Context, teleID int64, amount decimal.Decimal) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET balance_usdt = balance_usdt - $2
		WHERE tele_id = $1 AND balance_usdt >= $2
	`, teleID, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("insufficient balance")
	}
	return nil
}

// ListAll returns all users for admin.
func (r *UserRepo) ListAll(ctx context.Context) ([]models.User, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tele_id, username, balance_usdt, language, timezone, join_date, is_admin, is_banned
		FROM users ORDER BY join_date DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.TeleID, &u.Username, &u.BalanceUSDT, &u.Language, &u.Timezone, &u.JoinDate, &u.IsAdmin, &u.IsBanned); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// Search returns users filtered by telegram ID or username (admin).
func (r *UserRepo) Search(ctx context.Context, keyword string) ([]models.User, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tele_id, username, balance_usdt, language, timezone, join_date, is_admin, is_banned
		FROM users
		WHERE username ILIKE '%' || $1 || '%'
		   OR CAST(tele_id AS TEXT) ILIKE '%' || $1 || '%'
		ORDER BY join_date DESC
	`, keyword)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.TeleID, &u.Username, &u.BalanceUSDT, &u.Language, &u.Timezone, &u.JoinDate, &u.IsAdmin, &u.IsBanned); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// SetBanned sets user banned state.
func (r *UserRepo) SetBanned(ctx context.Context, teleID int64, banned bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET is_banned = $2 WHERE tele_id = $1`, teleID, banned)
	return err
}

// UserLang holds a user's tele_id, language and timezone for broadcast.
type UserLang struct {
	TeleID   int64
	Language string
	Timezone string
}

// ListAllUserLangs returns tele_id, language and timezone for all users (for broadcast).
func (r *UserRepo) ListAllUserLangs(ctx context.Context) ([]UserLang, error) {
	rows, err := r.pool.Query(ctx, `SELECT tele_id, language, timezone FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserLang
	for rows.Next() {
		var u UserLang
		if err := rows.Scan(&u.TeleID, &u.Language, &u.Timezone); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// UpdateTimezone sets user timezone.
func (r *UserRepo) UpdateTimezone(ctx context.Context, teleID int64, tz string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET timezone = $2 WHERE tele_id = $1`, teleID, tz)
	return err
}
