package crypto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"optimus-be/internal/infra/crypto"
)

func TestHashPassword_VerifiesWithCompare(t *testing.T) {
	h, err := crypto.HashPassword("s3cret", 4)
	require.NoError(t, err)
	require.NotEqual(t, "s3cret", h)
	require.NoError(t, crypto.ComparePassword(h, "s3cret"))
}

func TestComparePassword_RejectsWrong(t *testing.T) {
	h, _ := crypto.HashPassword("s3cret", 4)
	require.Error(t, crypto.ComparePassword(h, "wrong"))
}

func TestHashPassword_DistinctHashesForSameInput(t *testing.T) {
	h1, _ := crypto.HashPassword("s3cret", 4)
	h2, _ := crypto.HashPassword("s3cret", 4)
	require.NotEqual(t, h1, h2)
}

func TestHashPassword_RejectsEmpty(t *testing.T) {
	_, err := crypto.HashPassword("", 4)
	require.Error(t, err)
}
