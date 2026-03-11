package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateLoginToken creates a short-lived JWT token for admin login via bot link.
// This token is only valid for a few minutes, preventing reuse.
func GenerateLoginToken(teleID int64, secret string, expiry time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"tele_id": teleID,
		"purpose": "admin_login",
		"exp":     time.Now().Add(expiry).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateLoginToken verifies a login token and returns the tele_id if valid.
func ValidateLoginToken(tokenStr string, secret string) (int64, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return 0, fmt.Errorf("invalid or expired token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid token claims")
	}

	// Verify this token is specifically for login
	purpose, _ := claims["purpose"].(string)
	if purpose != "admin_login" {
		return 0, fmt.Errorf("token is not a login token")
	}

	teleIDf, ok := claims["tele_id"].(float64)
	if !ok {
		return 0, fmt.Errorf("missing tele_id in token")
	}

	return int64(teleIDf), nil
}

// GenerateSessionToken creates a longer-lived JWT token for admin session (stored in cookie).
func GenerateSessionToken(teleID int64, secret string, expiryHours int) (string, error) {
	claims := jwt.MapClaims{
		"tele_id": teleID,
		"purpose": "admin_session",
		"exp":     time.Now().Add(time.Duration(expiryHours) * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateSessionToken verifies a session token and returns the tele_id if valid.
func ValidateSessionToken(tokenStr string, secret string) (int64, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return 0, fmt.Errorf("invalid or expired token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid token claims")
	}

	purpose, _ := claims["purpose"].(string)
	if purpose != "admin_session" {
		return 0, fmt.Errorf("token is not a session token")
	}

	teleIDf, ok := claims["tele_id"].(float64)
	if !ok {
		return 0, fmt.Errorf("missing tele_id in token")
	}

	return int64(teleIDf), nil
}
