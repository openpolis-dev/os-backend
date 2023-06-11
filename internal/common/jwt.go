package common

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// JwtClaim payload of Jsonwebtoken
type JwtClaim[DATA any] struct {
	*jwt.RegisteredClaims
	Data *DATA
}

func GenerateJwtToken[DATA any](claimData *DATA, exp time.Duration, secret string) (token string, tokenExp int64, err error) {
	t := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		JwtClaim[DATA]{
			RegisteredClaims: &jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(exp)),
			},
			Data: claimData,
		},
	)
	token, err = t.SignedString([]byte(secret))
	return token, int64(exp / time.Second), err
}

func ValidateJwtToken[DATA any](token string, secret string) (*DATA, error) {
	// parse token
	t, err := jwt.ParseWithClaims(token, &JwtClaim[DATA]{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	// verify token
	if claims, ok := t.Claims.(*JwtClaim[DATA]); ok && t.Valid {
		return claims.Data, nil
	} else {
		return nil, errors.New("token invalidate")
	}
}
