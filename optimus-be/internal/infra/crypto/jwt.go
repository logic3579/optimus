package crypto

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	UserID uint64 `json:"uid"`
	// JTI is the JWT ID; stored in RegisteredClaims.ID ("jti") on the wire.
	JTI string `json:"-"`
}

type fullClaims struct {
	JWTClaims
	jwt.RegisteredClaims
}

type JWTSigner struct {
	secret []byte
}

func NewJWTSigner(secret string) *JWTSigner {
	return &JWTSigner{secret: []byte(secret)}
}

func (s *JWTSigner) Sign(c JWTClaims, ttl time.Duration) (string, error) {
	now := time.Now()
	fc := fullClaims{
		JWTClaims: c,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        c.JTI,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, fc)
	return tok.SignedString(s.secret)
}

func (s *JWTSigner) Verify(raw string) (*JWTClaims, error) {
	parsed, err := jwt.ParseWithClaims(raw, &fullClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := parsed.Claims.(*fullClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	// Restore JTI from RegisteredClaims.ID (standard "jti" claim).
	c.JWTClaims.JTI = c.RegisteredClaims.ID
	return &c.JWTClaims, nil
}
