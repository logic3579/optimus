// vault-keygen prints a fresh 32-byte base64-encoded master key to stdout.
// Usage:
//
//	$ optimus-vault-keygen > .vault-key
//	$ echo "OPTIMUS_VAULT_MASTER_KEY=$(optimus-vault-keygen)" >> .env
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := generate(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "vault-keygen:", err)
		os.Exit(1)
	}
}

func generate(w io.Writer) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("read random: %w", err)
	}
	enc := base64.StdEncoding.EncodeToString(key)
	_, err := fmt.Fprintln(w, enc)
	return err
}
