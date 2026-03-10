package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/nguyenan362/bot-shop-go/internal/models"
	"github.com/rs/zerolog/log"
)

const (
	binanceBaseURL     = "https://api.binance.com"
	depositHistoryPath = "/sapi/v1/capital/deposit/hisrec"
	depositAddressPath = "/sapi/v1/capital/deposit/address"
)

// DepositRecord represents a single deposit from Binance deposit history API.
type DepositRecord struct {
	ID           string `json:"id"`
	Amount       string `json:"amount"`
	Coin         string `json:"coin"`
	Network      string `json:"network"`
	Status       int    `json:"status"` // 0:pending, 6:credited, 1:success
	Address      string `json:"address"`
	AddressTag   string `json:"addressTag"` // MEMO field — used to identify Telegram user
	TxID         string `json:"txId"`
	InsertTime   int64  `json:"insertTime"`
	ConfirmTimes string `json:"confirmTimes"`
}

// DepositAddress represents a Binance deposit address response.
type DepositAddress struct {
	Address string `json:"address"`
	Tag     string `json:"tag"`
	Coin    string `json:"coin"`
	URL     string `json:"url"`
}

// Client handles Binance API calls.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new Binance API client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetDepositHistory fetches USDT deposit history from Binance.
// Only returns successful deposits (status=1).
// startTime is in milliseconds; if 0, returns recent deposits.
func (c *Client) GetDepositHistory(ctx context.Context, cfg *models.BinanceConfig, startTime int64) ([]DepositRecord, error) {
	params := url.Values{}
	params.Set("coin", "USDT")
	params.Set("status", "1") // 1 = success
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	params.Set("limit", "100")
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	// HMAC-SHA256 signature
	signature := signHMAC(cfg.SecretKey, params.Encode())
	params.Set("signature", signature)

	reqURL := binanceBaseURL + depositHistoryPath + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	log.Debug().Str("response", string(body)).Int("status", resp.StatusCode).Msg("binance deposit history response")

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("binance API error (status %d): %s", resp.StatusCode, string(body))
	}

	var records []DepositRecord
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("unmarshal deposit history: %w", err)
	}

	return records, nil
}

// GetDepositAddress fetches the deposit address for a coin/network from Binance.
func (c *Client) GetDepositAddress(ctx context.Context, cfg *models.BinanceConfig, coin, network string) (*DepositAddress, error) {
	params := url.Values{}
	params.Set("coin", coin)
	if network != "" {
		params.Set("network", network)
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	signature := signHMAC(cfg.SecretKey, params.Encode())
	params.Set("signature", signature)

	reqURL := binanceBaseURL + depositAddressPath + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	log.Debug().Str("response", string(body)).Int("status", resp.StatusCode).Msg("binance deposit address response")

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("binance API error (status %d): %s", resp.StatusCode, string(body))
	}

	var addr DepositAddress
	if err := json.Unmarshal(body, &addr); err != nil {
		return nil, fmt.Errorf("unmarshal deposit address: %w", err)
	}

	return &addr, nil
}

// signHMAC creates HMAC-SHA256 signature for Binance API authentication.
func signHMAC(secret, data string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}
