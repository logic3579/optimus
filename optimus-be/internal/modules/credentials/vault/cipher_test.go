package vault

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	key := make([]byte, KeyLen)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	c, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestCipher_RoundTrip_VariousSizes(t *testing.T) {
	c := newTestCipher(t)
	for _, n := range []int{0, 1, 17, 1024, 1024 * 1024} {
		pt := make([]byte, n)
		if _, err := rand.Read(pt); err != nil {
			t.Fatal(err)
		}
		ct, err := c.Seal(pt)
		if err != nil {
			t.Fatalf("Seal(%d): %v", n, err)
		}
		got, err := c.Open(ct)
		if err != nil {
			t.Fatalf("Open(%d): %v", n, err)
		}
		if !bytes.Equal(pt, got) {
			t.Errorf("size=%d round-trip mismatch", n)
		}
	}
}

func TestCipher_SealProducesDifferentCiphertexts(t *testing.T) {
	c := newTestCipher(t)
	pt := []byte("hello")
	a, _ := c.Seal(pt)
	b, _ := c.Seal(pt)
	if bytes.Equal(a, b) {
		t.Error("two seals of same plaintext are identical — nonce reuse?")
	}
}

func TestCipher_OpenRejectsTampering(t *testing.T) {
	c := newTestCipher(t)
	ct, _ := c.Seal([]byte("secret"))
	ct[len(ct)-1] ^= 0x01 // flip a tag bit
	if _, err := c.Open(ct); err == nil {
		t.Error("expected open to reject tampered ciphertext")
	}
}

func TestCipher_OpenRejectsShortInput(t *testing.T) {
	c := newTestCipher(t)
	if _, err := c.Open(make([]byte, 5)); err == nil {
		t.Error("expected error on short input")
	}
}

func TestCipher_OpenRejectsWrongKey(t *testing.T) {
	a := newTestCipher(t)
	b := newTestCipher(t)
	ct, _ := a.Seal([]byte("data"))
	if _, err := b.Open(ct); err == nil {
		t.Error("expected open with wrong key to fail")
	}
}

func TestNewCipher_RejectsWrongKeyLen(t *testing.T) {
	if _, err := NewCipher(make([]byte, 16)); err == nil {
		t.Error("expected error on 16-byte key")
	}
}
