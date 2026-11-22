package utils

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"researchhub-backend/config"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateJWT(userID, email, fullName, role string) (string, time.Time, error) {
	secret := "dev_only_researchhub_jwt_secret_key_2026"
	expiryHours := 24
	if config.Cfg != nil {
		if config.Cfg.JWTSecret != "" {
			secret = config.Cfg.JWTSecret
		}
		if config.Cfg.JWTExpiryHours > 0 {
			expiryHours = config.Cfg.JWTExpiryHours
		}
	}

	expiresAt := time.Now().Add(time.Duration(expiryHours) * time.Hour)
	claims := JWTClaims{
		UserID:   userID,
		Email:    email,
		FullName: fullName,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "ResearchHub",
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	return signed, expiresAt, err
}

func ValidateJWT(tokenString string) (*JWTClaims, error) {
	secret := "dev_only_researchhub_jwt_secret_key_2026"
	if config.Cfg != nil && config.Cfg.JWTSecret != "" {
		secret = config.Cfg.JWTSecret
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}

func GenerateSecureToken(byteLen int) string {
	b := make([]byte, byteLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
