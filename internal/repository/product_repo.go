package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nguyenan362/bot-shop-go/internal/models"
	"github.com/shopspring/decimal"
)

// ProductRepo handles product database operations.
type ProductRepo struct {
	pool *pgxpool.Pool
}

func NewProductRepo(pool *pgxpool.Pool) *ProductRepo {
	return &ProductRepo{pool: pool}
}

// ListActive returns all active products.
func (r *ProductRepo) ListActive(ctx context.Context) ([]models.Product, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name_vi, name_en, price_usdt, stock, description_vi, description_en, active, created_at
		FROM products WHERE active = true ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.NameVI, &p.NameEN, &p.PriceUSDT, &p.Stock,
			&p.DescriptionVI, &p.DescriptionEN, &p.Active, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, nil
}

// ListAll returns all products including inactive (admin).
func (r *ProductRepo) ListAll(ctx context.Context) ([]models.Product, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name_vi, name_en, price_usdt, stock, description_vi, description_en, active, created_at
		FROM products ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.NameVI, &p.NameEN, &p.PriceUSDT, &p.Stock,
			&p.DescriptionVI, &p.DescriptionEN, &p.Active, &p.CreatedAt); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, nil
}

// GetByID returns a product by ID.
func (r *ProductRepo) GetByID(ctx context.Context, id int) (*models.Product, error) {
	p := &models.Product{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name_vi, name_en, price_usdt, stock, description_vi, description_en, active, created_at
		FROM products WHERE id = $1
	`, id).Scan(&p.ID, &p.NameVI, &p.NameEN, &p.PriceUSDT, &p.Stock,
		&p.DescriptionVI, &p.DescriptionEN, &p.Active, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Create inserts a new product.
func (r *ProductRepo) Create(ctx context.Context, p *models.Product) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO products (name_vi, name_en, price_usdt, stock, description_vi, description_en, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, p.NameVI, p.NameEN, p.PriceUSDT, p.Stock, p.DescriptionVI, p.DescriptionEN, p.Active).Scan(&p.ID)
}

// Update modifies a product.
func (r *ProductRepo) Update(ctx context.Context, p *models.Product) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE products SET name_vi=$2, name_en=$3, price_usdt=$4, stock=$5,
		       description_vi=$6, description_en=$7, active=$8
		WHERE id=$1
	`, p.ID, p.NameVI, p.NameEN, p.PriceUSDT, p.Stock, p.DescriptionVI, p.DescriptionEN, p.Active)
	return err
}

// Delete removes a product.
func (r *ProductRepo) Delete(ctx context.Context, id int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM products WHERE id=$1`, id)
	return err
}

// DeductStock decreases stock atomically.
func (r *ProductRepo) DeductStock(ctx context.Context, productID int, qty int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE products SET stock = stock - $2
		WHERE id = $1 AND stock >= $2
	`, productID, qty)
	return err
}

// IncrementStock increases stock count.
func (r *ProductRepo) IncrementStock(ctx context.Context, productID int, qty int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE products SET stock = stock + $2 WHERE id = $1
	`, productID, qty)
	return err
}

// ---- Product Accounts ----

// AddAccounts inserts multiple accounts for a product.
func (r *ProductRepo) AddAccounts(ctx context.Context, productID int, accounts []string) (int, error) {
	count := 0
	for _, acc := range accounts {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO product_accounts (product_id, account_data) VALUES ($1, $2)
		`, productID, acc)
		if err != nil {
			return count, err
		}
		count++
	}
	// Update stock
	err := r.IncrementStock(ctx, productID, count)
	return count, err
}

// ClaimAccounts marks N unused accounts as used and assigns them to an order.
func (r *ProductRepo) ClaimAccounts(ctx context.Context, productID int, orderID int64, qty int) ([]models.ProductAccount, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE product_accounts
		SET used = true, order_id = $3
		WHERE id IN (
			SELECT id FROM product_accounts
			WHERE product_id = $1 AND used = false
			ORDER BY id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, product_id, account_data, used, order_id, created_at
	`, productID, qty, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.ProductAccount
	for rows.Next() {
		var a models.ProductAccount
		if err := rows.Scan(&a.ID, &a.ProductID, &a.AccountData, &a.Used, &a.OrderID, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// CountAvailable returns the number of unused accounts for a product.
func (r *ProductRepo) CountAvailable(ctx context.Context, productID int) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM product_accounts
		WHERE product_id = $1 AND used = false
	`, productID).Scan(&count)
	return count, err
}

// ListAccounts returns all accounts for a product with optional filter.
func (r *ProductRepo) ListAccounts(ctx context.Context, productID int, filter string) ([]models.ProductAccount, error) {
	query := `SELECT id, product_id, account_data, used, COALESCE(order_id, 0), created_at
	          FROM product_accounts WHERE product_id = $1`
	switch filter {
	case "available":
		query += " AND used = false"
	case "used":
		query += " AND used = true"
	}
	query += " ORDER BY id DESC"

	rows, err := r.pool.Query(ctx, query, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []models.ProductAccount
	for rows.Next() {
		var a models.ProductAccount
		if err := rows.Scan(&a.ID, &a.ProductID, &a.AccountData, &a.Used, &a.OrderID, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// DeleteAccount removes a single unused account.
func (r *ProductRepo) DeleteAccount(ctx context.Context, accountID int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM product_accounts WHERE id = $1 AND used = false`, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("account not found or already used")
	}
	return nil
}

// DeleteAllUnusedAccounts removes all unused accounts for a product and adjusts stock.
func (r *ProductRepo) DeleteAllUnusedAccounts(ctx context.Context, productID int) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM product_accounts WHERE product_id = $1 AND used = false`, productID)
	if err != nil {
		return 0, err
	}
	deleted := int(tag.RowsAffected())
	if deleted > 0 {
		_, err = r.pool.Exec(ctx, `UPDATE products SET stock = stock - $2 WHERE id = $1 AND stock >= $2`, productID, deleted)
	}
	return deleted, err
}

// CountUsed returns the number of used accounts for a product.
func (r *ProductRepo) CountUsed(ctx context.Context, productID int) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM product_accounts
		WHERE product_id = $1 AND used = true
	`, productID).Scan(&count)
	return count, err
}

// ---- Binance Config ----

// GetBinanceConfig returns Binance API configuration.
func (r *ProductRepo) GetBinanceConfig(ctx context.Context) (*models.BinanceConfig, error) {
	bc := &models.BinanceConfig{}
	err := r.pool.QueryRow(ctx, `
		SELECT api_key, secret_key, 
		       COALESCE(deposit_address,''), COALESCE(deposit_network,'')
		FROM binance_config WHERE id = 1
	`).Scan(&bc.APIKey, &bc.SecretKey, &bc.DepositAddress, &bc.DepositNetwork)
	if err != nil {
		return nil, err
	}
	return bc, nil
}

// UpdateBinanceConfig updates Binance API configuration.
func (r *ProductRepo) UpdateBinanceConfig(ctx context.Context, bc *models.BinanceConfig) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE binance_config SET api_key=$1, secret_key=$2,
		       deposit_address=$3, deposit_network=$4, updated_at=NOW()
		WHERE id=1
	`, bc.APIKey, bc.SecretKey, bc.DepositAddress, bc.DepositNetwork)
	return err
}

// ---- Price helper ----

// CalcTotal returns price * quantity.
func CalcTotal(price decimal.Decimal, qty int) decimal.Decimal {
	return price.Mul(decimal.NewFromInt(int64(qty)))
}
