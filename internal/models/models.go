package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// User represents a Telegram user.
type User struct {
	TeleID      int64           `json:"tele_id"`
	Username    string          `json:"username"`
	BalanceUSDT decimal.Decimal `json:"balance_usdt"`
	Language    string          `json:"language"`
	Timezone    string          `json:"timezone"`
	JoinDate    time.Time       `json:"join_date"`
	IsAdmin     bool            `json:"is_admin"`
}

// Product represents an account type for sale.
type Product struct {
	ID            int             `json:"id"`
	NameVI        string          `json:"name_vi"`
	NameEN        string          `json:"name_en"`
	PriceUSDT     decimal.Decimal `json:"price_usdt"`
	Stock         int             `json:"stock"`
	DescriptionVI string          `json:"description_vi"`
	DescriptionEN string          `json:"description_en"`
	Active        bool            `json:"active"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Name returns the product name based on language.
func (p Product) Name(lang string) string {
	if lang == "en" {
		return p.NameEN
	}
	return p.NameVI
}

// Description returns the product description based on language.
func (p Product) Description(lang string) string {
	if lang == "en" {
		return p.DescriptionEN
	}
	return p.DescriptionVI
}

// ProductAccount represents an actual account in stock.
type ProductAccount struct {
	ID            int       `json:"id"`
	ProductID     int       `json:"product_id"`
	AccountData   string    `json:"account_data"`
	Used          bool      `json:"used"`
	OrderID       int64     `json:"order_id"`
	BuyerUsername string    `json:"buyer_username"`
	BuyerTeleID   int64     `json:"buyer_tele_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// Order represents a purchase order.
type Order struct {
	ID         int64           `json:"id"`
	UserTeleID int64           `json:"user_tele_id"`
	Username   string          `json:"username"`
	ProductID  int             `json:"product_id"`
	Quantity   int             `json:"quantity"`
	TotalUSDT  decimal.Decimal `json:"total_usdt"`
	Status     string          `json:"status"` // pending, success, failed
	CreatedAt  time.Time       `json:"created_at"`
}

// Deposit represents a USDT deposit detected from Binance.
type Deposit struct {
	ID              int64           `json:"id"`
	UserTeleID      int64           `json:"user_tele_id"`
	Username        string          `json:"username"`
	TxID            string          `json:"tx_id"`
	MerchantTradeNo string          `json:"merchant_trade_no"` // kept for backward compat
	AmountUSDT      decimal.Decimal `json:"amount_usdt"`
	Status          string          `json:"status"` // paid, unmatched
	Network         string          `json:"network"`
	PayURL          string          `json:"pay_url"` // kept for backward compat
	CreatedAt       time.Time       `json:"created_at"`
	PaidAt          *time.Time      `json:"paid_at,omitempty"`
}

// Note represents an announcement/notice for users.
type Note struct {
	ID        int    `json:"id"`
	ContentVI string `json:"content_vi"`
	ContentEN string `json:"content_en"`
	Active    bool   `json:"active"`
}

// Content returns the note content based on language.
func (n Note) Content(lang string) string {
	if lang == "en" {
		return n.ContentEN
	}
	return n.ContentVI
}

// BinanceConfig stores Binance API config for deposit monitoring.
type BinanceConfig struct {
	APIKey         string `json:"api_key"`
	SecretKey      string `json:"secret_key"`
	DepositAddress string `json:"deposit_address"` // Wallet address for users to send USDT
	DepositNetwork string `json:"deposit_network"` // Network: TON, BEP2, BSC, etc.
}
