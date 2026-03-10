package service

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nguyenan362/bot-shop-go/internal/binance"
	"github.com/nguyenan362/bot-shop-go/internal/models"
	"github.com/nguyenan362/bot-shop-go/internal/repository"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// ShopService contains all business logic.
type ShopService struct {
	UserRepo    *repository.UserRepo
	ProductRepo *repository.ProductRepo
	OrderRepo   *repository.OrderRepo
	DepositRepo *repository.DepositRepo
	NoteRepo    *repository.NoteRepo
	Redis       *redis.Client
	Binance     *binance.Client
}

// NewShopService creates a new service.
func NewShopService(
	userRepo *repository.UserRepo,
	productRepo *repository.ProductRepo,
	orderRepo *repository.OrderRepo,
	depositRepo *repository.DepositRepo,
	noteRepo *repository.NoteRepo,
	rdb *redis.Client,
	bp *binance.Client,
) *ShopService {
	return &ShopService{
		UserRepo:    userRepo,
		ProductRepo: productRepo,
		OrderRepo:   orderRepo,
		DepositRepo: depositRepo,
		NoteRepo:    noteRepo,
		Redis:       rdb,
		Binance:     bp,
	}
}

// BuyResult holds the result of a purchase.
type BuyResult struct {
	Order    *models.Order
	Accounts []models.ProductAccount
	FileData []byte
	FileName string
}

// BuyAccounts processes a purchase: check balance, deduct, claim accounts, generate file.
func (s *ShopService) BuyAccounts(ctx context.Context, teleID int64, productID int, qty int) (*BuyResult, error) {
	// 1. Get product
	product, err := s.ProductRepo.GetByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if !product.Active {
		return nil, fmt.Errorf("product is not active")
	}

	// 2. Calculate total
	total := repository.CalcTotal(product.PriceUSDT, qty)

	// 3. Check available stock
	available, err := s.ProductRepo.CountAvailable(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("count available: %w", err)
	}
	if available < qty {
		return nil, fmt.Errorf("out_of_stock:%d", available)
	}

	// 4. Acquire distributed lock
	lockKey := fmt.Sprintf("lock:buy:%d:%d", teleID, productID)
	locked, err := s.Redis.SetNX(ctx, lockKey, "1", 30*time.Second).Result()
	if err != nil {
		return nil, fmt.Errorf("redis lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("purchase already in progress")
	}
	defer s.Redis.Del(ctx, lockKey)

	// 5. Deduct balance
	if err := s.UserRepo.DeductBalance(ctx, teleID, total); err != nil {
		return nil, fmt.Errorf("deduct_balance:%s", err.Error())
	}

	// 6. Create order
	order := &models.Order{
		UserTeleID: teleID,
		ProductID:  productID,
		Quantity:   qty,
		TotalUSDT:  total,
		Status:     "pending",
	}
	if err := s.OrderRepo.Create(ctx, order); err != nil {
		// Refund balance on failure
		_ = s.UserRepo.AddBalance(ctx, teleID, total)
		return nil, fmt.Errorf("create order: %w", err)
	}

	// 7. Claim accounts
	accounts, err := s.ProductRepo.ClaimAccounts(ctx, productID, order.ID, qty)
	if err != nil || len(accounts) < qty {
		// Refund
		_ = s.UserRepo.AddBalance(ctx, teleID, total)
		_ = s.OrderRepo.UpdateStatus(ctx, order.ID, "failed")
		return nil, fmt.Errorf("claim accounts: insufficient stock")
	}

	// 8. Update order status
	_ = s.OrderRepo.UpdateStatus(ctx, order.ID, "success")

	// 9. Deduct stock count
	_ = s.ProductRepo.DeductStock(ctx, productID, qty)

	// 10. Generate TXT file
	fileData, fileName := generateAccountFile(order.ID, accounts)

	log.Info().
		Int64("user", teleID).
		Int64("order", order.ID).
		Int("qty", qty).
		Str("total", total.String()).
		Msg("purchase successful")

	return &BuyResult{
		Order:    order,
		Accounts: accounts,
		FileData: fileData,
		FileName: fileName,
	}, nil
}

// generateAccountFile creates a TXT file with account data.
func generateAccountFile(orderID int64, accounts []models.ProductAccount) ([]byte, string) {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("=== Order #%d ===\n", orderID))
	buf.WriteString(fmt.Sprintf("Date: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	buf.WriteString(fmt.Sprintf("Quantity: %d\n", len(accounts)))
	buf.WriteString("========================\n\n")

	for i, acc := range accounts {
		buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, acc.AccountData))
	}

	fileName := fmt.Sprintf("order_%d_%s.txt", orderID, time.Now().Format("20060102_150405"))
	return buf.Bytes(), fileName
}

// GetDepositInfo returns the Binance deposit address and network for a user.
func (s *ShopService) GetDepositInfo(ctx context.Context) (*models.BinanceConfig, error) {
	cfg, err := s.ProductRepo.GetBinanceConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("binance config: %w", err)
	}
	if cfg.APIKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("binance_not_configured")
	}
	if cfg.DepositAddress == "" {
		return nil, fmt.Errorf("binance_not_configured")
	}
	return cfg, nil
}

// VerifyAndCreditDeposit verifies a user-submitted TxID against Binance deposit history,
// then credits the user if the deposit is valid and not already processed.
// Returns (amount, error).
func (s *ShopService) VerifyAndCreditDeposit(ctx context.Context, teleID int64, txID string) (decimal.Decimal, error) {
	// 1. Check if this TxID/ID was already claimed (status=paid)
	existing, _ := s.DepositRepo.GetByTxID(ctx, txID)
	if existing != nil && existing.Status == "paid" {
		return decimal.Zero, fmt.Errorf("txid_already_used")
	}

	// 2. Acquire lock to prevent race condition
	lockKey := fmt.Sprintf("lock:deposit:%s", txID)
	locked, _ := s.Redis.SetNX(ctx, lockKey, "1", 5*time.Minute).Result()
	if !locked {
		return decimal.Zero, fmt.Errorf("deposit_processing")
	}
	defer s.Redis.Del(ctx, lockKey)

	// 3. Get Binance config
	cfg, err := s.ProductRepo.GetBinanceConfig(ctx)
	if err != nil || cfg.APIKey == "" {
		return decimal.Zero, fmt.Errorf("binance_not_configured")
	}

	// 4. Search this TxID in Binance deposit history
	// Check the last 7 days
	startTime := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	deposits, err := s.Binance.GetDepositHistory(ctx, cfg, startTime)
	if err != nil {
		return decimal.Zero, fmt.Errorf("binance_api_error")
	}

	var matched *binance.DepositRecord
	for i, d := range deposits {
		// Match by:
		// 1. Exact TxHash match
		// 2. Exact Binance internal ID match
		// 3. Off-chain transfer: txId is "Off-chain transfer <ID>" — match the numeric ID part
		// 4. User might send the full "Off-chain transfer ..." string
		if d.TxID == txID || d.ID == txID ||
			strings.Contains(d.TxID, txID) ||
			strings.Contains(txID, d.ID) {
			matched = &deposits[i]
			break
		}
	}

	if matched == nil {
		return decimal.Zero, fmt.Errorf("txid_not_found")
	}

	// Use the actual blockchain TxID for dedup; fall back to Binance internal ID if empty
	actualTxID := matched.TxID
	if actualTxID == "" {
		actualTxID = matched.ID
	}

	// Double-check: the actual TxID might differ from what user submitted (user may have sent internal ID)
	if actualTxID != txID {
		existByActual, _ := s.DepositRepo.GetByTxID(ctx, actualTxID)
		if existByActual != nil && existByActual.Status == "paid" {
			return decimal.Zero, fmt.Errorf("txid_already_used")
		}
		// Also update existing reference
		if existByActual != nil {
			existing = existByActual
		}
	}

	// 5. Validate: must be USDT and status=1 (success)
	if matched.Coin != "USDT" {
		return decimal.Zero, fmt.Errorf("not_usdt")
	}
	if matched.Status != 1 {
		return decimal.Zero, fmt.Errorf("deposit_not_confirmed")
	}

	amount, _ := decimal.NewFromString(matched.Amount)
	if amount.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("invalid_amount")
	}

	// 6. Verify deposit address matches our configured address
	if cfg.DepositAddress != "" && matched.Address != cfg.DepositAddress {
		return decimal.Zero, fmt.Errorf("wrong_address")
	}

	// 7. Credit user balance
	if err := s.UserRepo.AddBalance(ctx, teleID, amount); err != nil {
		return decimal.Zero, fmt.Errorf("add balance: %w", err)
	}

	// 8. Save or update deposit record (use actualTxID for proper dedup)
	if existing != nil {
		// Poller already indexed this deposit as "pending" — update to "paid" with user info
		_ = s.DepositRepo.UpdateClaimed(ctx, existing.TxID, teleID, "paid")
	} else {
		deposit := &models.Deposit{
			UserTeleID: teleID,
			TxID:       actualTxID,
			AmountUSDT: amount,
			Status:     "paid",
			Network:    matched.Network,
		}
		if err := s.DepositRepo.Create(ctx, deposit); err != nil {
			log.Error().Err(err).Str("txId", actualTxID).Msg("save deposit record failed")
		}
	}

	log.Info().
		Int64("user", teleID).
		Str("txId", actualTxID).
		Str("binanceId", matched.ID).
		Str("amount", amount.String()).
		Str("network", matched.Network).
		Msg("deposit verified and credited via TxID")

	return amount, nil
}

// PollBinanceDeposits runs a background loop that checks Binance deposit history every 60s
// and saves new deposits for record-keeping. Primary crediting is via user TxID submission.
// notifyFn is called for deposits that can be auto-matched (unlikely without memo networks).
func (s *ShopService) PollBinanceDeposits(ctx context.Context, notifyFn func(teleID int64, amount decimal.Decimal)) {
	// Wait before first poll
	time.Sleep(10 * time.Second)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	s.syncNewDeposits(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("deposit poller stopped")
			return
		case <-ticker.C:
			s.syncNewDeposits(ctx)
		}
	}
}

// syncNewDeposits fetches recent Binance deposits and saves unprocessed ones for admin visibility.
func (s *ShopService) syncNewDeposits(ctx context.Context) {
	cfg, err := s.ProductRepo.GetBinanceConfig(ctx)
	if err != nil || cfg.APIKey == "" || cfg.SecretKey == "" {
		return
	}

	lastCheckMs := s.getLastDepositCheckTime(ctx)

	deposits, err := s.Binance.GetDepositHistory(ctx, cfg, lastCheckMs)
	if err != nil {
		log.Error().Err(err).Msg("poll binance deposits failed")
		return
	}

	if len(deposits) == 0 {
		return
	}

	var latestTime int64

	for _, d := range deposits {
		if d.InsertTime > latestTime {
			latestTime = d.InsertTime
		}

		// Skip if already recorded (check both TxID and Binance internal ID)
		txIDToStore := d.TxID
		if txIDToStore == "" {
			txIDToStore = d.ID
		}
		exists, _ := s.DepositRepo.ExistsByTxID(ctx, txIDToStore)
		if exists {
			continue
		}
		// Also check by Binance internal ID if different
		if d.ID != "" && d.ID != txIDToStore {
			existsID, _ := s.DepositRepo.ExistsByTxID(ctx, d.ID)
			if existsID {
				continue
			}
		}

		amount, _ := decimal.NewFromString(d.Amount)
		if amount.LessThanOrEqual(decimal.Zero) {
			continue
		}

		// Save as "pending" — waiting for user to claim via TxID submission
		deposit := &models.Deposit{
			UserTeleID: 0,
			TxID:       txIDToStore,
			AmountUSDT: amount,
			Status:     "pending",
			Network:    d.Network,
		}
		_ = s.DepositRepo.Create(ctx, deposit)

		log.Info().
			Str("txId", txIDToStore).
			Str("binanceId", d.ID).
			Str("amount", d.Amount).
			Str("network", d.Network).
			Msg("new deposit synced from Binance (awaiting user claim)")
	}

	if latestTime > 0 {
		s.setLastDepositCheckTime(ctx, latestTime+1)
	}
}

// getLastDepositCheckTime returns the last deposit check time from Redis (in ms).
func (s *ShopService) getLastDepositCheckTime(ctx context.Context) int64 {
	val, err := s.Redis.Get(ctx, "binance:last_deposit_check").Result()
	if err != nil {
		// Default: check last 24 hours
		return time.Now().Add(-24 * time.Hour).UnixMilli()
	}
	ms, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Now().Add(-24 * time.Hour).UnixMilli()
	}
	return ms
}

// setLastDepositCheckTime stores the last deposit check time in Redis (in ms).
func (s *ShopService) setLastDepositCheckTime(ctx context.Context, ms int64) {
	s.Redis.Set(ctx, "binance:last_deposit_check", strconv.FormatInt(ms, 10), 0)
}

// SetUserState stores temporary user state in Redis (e.g., awaiting quantity input).
func (s *ShopService) SetUserState(ctx context.Context, teleID int64, state string, data string) error {
	key := fmt.Sprintf("state:%d", teleID)
	val := state + ":" + data
	return s.Redis.Set(ctx, key, val, 10*time.Minute).Err()
}

// GetUserState retrieves user state from Redis.
func (s *ShopService) GetUserState(ctx context.Context, teleID int64) (string, string, error) {
	key := fmt.Sprintf("state:%d", teleID)
	val, err := s.Redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	// Split "state:data"
	for i := 0; i < len(val); i++ {
		if val[i] == ':' {
			return val[:i], val[i+1:], nil
		}
	}
	return val, "", nil
}

// ClearUserState removes user state from Redis.
func (s *ShopService) ClearUserState(ctx context.Context, teleID int64) error {
	key := fmt.Sprintf("state:%d", teleID)
	return s.Redis.Del(ctx, key).Err()
}
