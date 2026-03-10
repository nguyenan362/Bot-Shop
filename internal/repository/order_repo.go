package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nguyenan362/bot-shop-go/internal/models"
	"github.com/shopspring/decimal"
)

// OrderRepo handles order database operations.
type OrderRepo struct {
	pool *pgxpool.Pool
}

func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo {
	return &OrderRepo{pool: pool}
}

// Create inserts a new order and returns its ID.
func (r *OrderRepo) Create(ctx context.Context, o *models.Order) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO orders (user_tele_id, product_id, quantity, total_usdt, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, o.UserTeleID, o.ProductID, o.Quantity, o.TotalUSDT, o.Status).Scan(&o.ID, &o.CreatedAt)
}

// UpdateStatus updates order status.
func (r *OrderRepo) UpdateStatus(ctx context.Context, orderID int64, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE orders SET status = $2 WHERE id = $1`, orderID, status)
	return err
}

// ListByUser returns orders for a specific user.
func (r *OrderRepo) ListByUser(ctx context.Context, teleID int64) ([]models.Order, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_tele_id, product_id, quantity, total_usdt, status, created_at
		FROM orders WHERE user_tele_id = $1 ORDER BY created_at DESC LIMIT 50
	`, teleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.UserTeleID, &o.ProductID, &o.Quantity, &o.TotalUSDT, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

// ListAll returns all orders (admin).
func (r *OrderRepo) ListAll(ctx context.Context, limit int) ([]models.Order, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_tele_id, product_id, quantity, total_usdt, status, created_at
		FROM orders ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		if err := rows.Scan(&o.ID, &o.UserTeleID, &o.ProductID, &o.Quantity, &o.TotalUSDT, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

// ---- Deposits ----

// DepositRepo handles deposit operations.
type DepositRepo struct {
	pool *pgxpool.Pool
}

func NewDepositRepo(pool *pgxpool.Pool) *DepositRepo {
	return &DepositRepo{pool: pool}
}

// Create inserts a new deposit record (from Binance deposit poller).
func (r *DepositRepo) Create(ctx context.Context, d *models.Deposit) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO deposits (user_tele_id, tx_id, amount_usdt, status, network, paid_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id, created_at
	`, d.UserTeleID, d.TxID, d.AmountUSDT, d.Status, d.Network).Scan(&d.ID, &d.CreatedAt)
}

// ExistsByTxID checks if a deposit with the given txId already exists.
// Also matches off-chain transfers where tx_id is "Off-chain transfer <ID>".
func (r *DepositRepo) ExistsByTxID(ctx context.Context, txID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM deposits 
			WHERE tx_id != '' AND (tx_id = $1 OR tx_id LIKE '%' || $1 || '%' OR $1 LIKE '%' || tx_id || '%')
		)
	`, txID).Scan(&exists)
	return exists, err
}

// UpdateClaimed updates a pending deposit to claimed status with user info.
func (r *DepositRepo) UpdateClaimed(ctx context.Context, txID string, teleID int64, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE deposits SET user_tele_id = $1, status = $2, paid_at = NOW()
		WHERE tx_id = $3 OR tx_id LIKE '%' || $3 || '%'
	`, teleID, status, txID)
	return err
}

// GetByTxID returns a deposit by Binance transaction ID.
// Also matches off-chain transfers where tx_id is "Off-chain transfer <ID>".
func (r *DepositRepo) GetByTxID(ctx context.Context, txID string) (*models.Deposit, error) {
	d := &models.Deposit{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_tele_id, COALESCE(tx_id,''), COALESCE(merchant_trade_no,''), amount_usdt, status, 
		       COALESCE(network,''), COALESCE(pay_url,''), created_at, paid_at
		FROM deposits WHERE tx_id = $1 OR tx_id LIKE '%' || $1 || '%' OR $1 LIKE '%' || tx_id || '%'
		ORDER BY created_at DESC LIMIT 1
	`, txID).Scan(&d.ID, &d.UserTeleID, &d.TxID, &d.MerchantTradeNo, &d.AmountUSDT,
		&d.Status, &d.Network, &d.PayURL, &d.CreatedAt, &d.PaidAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// ListAll returns recent deposits (admin).
func (r *DepositRepo) ListAll(ctx context.Context, limit int) ([]models.Deposit, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_tele_id, COALESCE(tx_id,''), COALESCE(merchant_trade_no,''), 
		       amount_usdt, status, COALESCE(network,''), COALESCE(pay_url,''), created_at, paid_at
		FROM deposits ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deposits []models.Deposit
	for rows.Next() {
		var d models.Deposit
		if err := rows.Scan(&d.ID, &d.UserTeleID, &d.TxID, &d.MerchantTradeNo, &d.AmountUSDT,
			&d.Status, &d.Network, &d.PayURL, &d.CreatedAt, &d.PaidAt); err != nil {
			return nil, err
		}
		deposits = append(deposits, d)
	}
	return deposits, nil
}

// ---- Notes ----

// NoteRepo handles note operations.
type NoteRepo struct {
	pool *pgxpool.Pool
}

func NewNoteRepo(pool *pgxpool.Pool) *NoteRepo {
	return &NoteRepo{pool: pool}
}

// ListActive returns active notes.
func (r *NoteRepo) ListActive(ctx context.Context) ([]models.Note, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, content_vi, content_en, active FROM notes WHERE active = true ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []models.Note
	for rows.Next() {
		var n models.Note
		if err := rows.Scan(&n.ID, &n.ContentVI, &n.ContentEN, &n.Active); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// ListAll returns all notes (admin).
func (r *NoteRepo) ListAll(ctx context.Context) ([]models.Note, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, content_vi, content_en, active FROM notes ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []models.Note
	for rows.Next() {
		var n models.Note
		if err := rows.Scan(&n.ID, &n.ContentVI, &n.ContentEN, &n.Active); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// Create inserts a new note.
func (r *NoteRepo) Create(ctx context.Context, n *models.Note) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO notes (content_vi, content_en, active) VALUES ($1, $2, $3) RETURNING id
	`, n.ContentVI, n.ContentEN, n.Active).Scan(&n.ID)
}

// Update modifies a note.
func (r *NoteRepo) Update(ctx context.Context, n *models.Note) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notes SET content_vi=$2, content_en=$3, active=$4 WHERE id=$1
	`, n.ID, n.ContentVI, n.ContentEN, n.Active)
	return err
}

// Delete removes a note.
func (r *NoteRepo) Delete(ctx context.Context, id int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notes WHERE id=$1`, id)
	return err
}

// ---- Stats ----

type Stats struct {
	TotalUsers    int             `json:"total_users"`
	TotalOrders   int             `json:"total_orders"`
	TotalRevenue  decimal.Decimal `json:"total_revenue"`
	TotalDeposits decimal.Decimal `json:"total_deposits"`
}

// GetStats returns dashboard statistics.
func GetStats(ctx context.Context, pool *pgxpool.Pool) (*Stats, error) {
	s := &Stats{}
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&s.TotalUsers)
	if err != nil {
		return nil, err
	}
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE status='success'`).Scan(&s.TotalOrders)
	if err != nil {
		return nil, err
	}
	err = pool.QueryRow(ctx, `SELECT COALESCE(SUM(total_usdt),0) FROM orders WHERE status='success'`).Scan(&s.TotalRevenue)
	if err != nil {
		return nil, err
	}
	err = pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount_usdt),0) FROM deposits WHERE status='paid'`).Scan(&s.TotalDeposits)
	if err != nil {
		return nil, err
	}
	return s, nil
}
