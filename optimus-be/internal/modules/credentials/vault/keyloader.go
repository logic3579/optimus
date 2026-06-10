// Package vault is the credentials-vault crypto core. It owns the master key
// and exposes only Seal/Open via the Cipher type. No other package in the
// service should import crypto/aes or crypto/cipher for credential material.
package vault

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// Source describes where the 32-byte master key comes from. Env wins; if Env
// is empty, File is consulted. If both are empty, LoadKey errors.
type Source struct {
	Env  string // base64-encoded key, length-32 after decode
	File string // path to a file containing either base64 or raw 32 bytes
}

const KeyLen = 32

// LoadKey resolves and returns the master key bytes. Always returns either a
// 32-byte slice or a non-nil error — never both.
func LoadKey(src Source) ([]byte, error) {
	if src.Env != "" {
		return decodeKey([]byte(src.Env), "env OPTIMUS_VAULT_MASTER_KEY")
	}
	if src.File != "" {
		raw, err := os.ReadFile(src.File)
		if err != nil {
			return nil, fmt.Errorf("vault: read master key file %q: %w", src.File, err)
		}
		return decodeKey(raw, fmt.Sprintf("file %q", src.File))
	}
	return nil, errors.New("vault: master key not configured (set OPTIMUS_VAULT_MASTER_KEY or OPTIMUS_VAULT_MASTER_KEY_FILE)")
}

// decodeKey accepts either base64-encoded 32 bytes OR raw 32 bytes.
// Trailing whitespace (common in files written via echo / shell redirect) is trimmed.
func decodeKey(input []byte, sourceLabel string) ([]byte, error) {
	trimmed := trimTrailingWS(input)
	if len(trimmed) == KeyLen {
		out := make([]byte, KeyLen)
		copy(out, trimmed)
		return out, nil
	}
	dec, err := base64.StdEncoding.DecodeString(string(trimmed))
	if err != nil {
		return nil, fmt.Errorf("vault: master key from %s is neither raw 32 bytes nor base64: %w", sourceLabel, err)
	}
	if len(dec) != KeyLen {
		return nil, fmt.Errorf("vault: master key from %s decoded to %d bytes, want %d", sourceLabel, len(dec), KeyLen)
	}
	return dec, nil
}

func trimTrailingWS(b []byte) []byte {
	for len(b) > 0 {
		last := b[len(b)-1]
		if last == '\n' || last == '\r' || last == ' ' || last == '\t' {
			b = b[:len(b)-1]
			continue
		}
		break
	}
	return b
}
