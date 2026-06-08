package crypto_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
)

const testSecret = "test_secret_must_be_at_least_32_bytes_!!"

func TestJWT_SignAndVerifyRoundTrip(t *testing.T) {
	signer := crypto.NewJWTSigner(testSecret)
	tok, err := signer.Sign(crypto.JWTClaims{UserID: 42, JTI: "j1"}, 5*time.Minute)
	require.NoError(t, err)
	require.Contains(t, tok, ".")

	claims, err := signer.Verify(tok)
	require.NoError(t, err)
	require.Equal(t, uint64(42), claims.UserID)
	require.Equal(t, "j1", claims.JTI)
}

func TestJWT_RejectsTamperedToken(t *testing.T) {
	signer := crypto.NewJWTSigner(testSecret)
	tok, _ := signer.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, 5*time.Minute)
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	tampered := parts[0] + "." + parts[1] + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	_, err := signer.Verify(tampered)
	require.Error(t, err)
}

func TestJWT_RejectsExpiredToken(t *testing.T) {
	signer := crypto.NewJWTSigner(testSecret)
	tok, _ := signer.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, -1*time.Second)
	_, err := signer.Verify(tok)
	require.Error(t, err)
}

func TestJWT_RejectsDifferentSecret(t *testing.T) {
	a := crypto.NewJWTSigner(testSecret)
	b := crypto.NewJWTSigner("different_secret_at_least_32_bytes_wxyz!")
	tok, _ := a.Sign(crypto.JWTClaims{UserID: 1, JTI: "j"}, 5*time.Minute)
	_, err := b.Verify(tok)
	require.Error(t, err)
}
