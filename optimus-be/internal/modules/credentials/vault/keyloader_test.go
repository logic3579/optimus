package vault

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKey_FromEnv_Base64(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	enc := base64.StdEncoding.EncodeToString(raw)

	got, err := LoadKey(Source{Env: enc})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("key mismatch: got %x want %x", got, raw)
	}
}

func TestLoadKey_FromFile_Base64(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(0xAA ^ i)
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	dir := t.TempDir()
	p := filepath.Join(dir, "key")
	if err := os.WriteFile(p, []byte(enc), 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(Source{File: p})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("key mismatch")
	}
}

func TestLoadKey_FromFile_RawBytes(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "key.bin")
	if err := os.WriteFile(p, raw, 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(Source{File: p})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("raw key mismatch")
	}
}

func TestLoadKey_FromFile_Base64WithTrailingNewline(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(0x55 ^ i)
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	dir := t.TempDir()
	p := filepath.Join(dir, "key")
	if err := os.WriteFile(p, []byte(enc+"\n"), 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(Source{File: p})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("trailing newline broke decoding")
	}
}

func TestLoadKey_EnvWins(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	dir := t.TempDir()
	p := filepath.Join(dir, "key")
	if err := os.WriteFile(p, []byte("wrong-content"), 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := LoadKey(Source{Env: enc, File: p})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("env did not win")
	}
}

func TestLoadKey_NoSource_Fails(t *testing.T) {
	_, err := LoadKey(Source{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadKey_WrongLength_Fails(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := LoadKey(Source{Env: short})
	if err == nil {
		t.Fatal("expected length error")
	}
}

func TestLoadKey_FileMissing_Fails(t *testing.T) {
	_, err := LoadKey(Source{File: "/nonexistent/path/xyz"})
	if err == nil {
		t.Fatal("expected file error")
	}
}

func TestLoadKey_GarbageNotBase64_Fails(t *testing.T) {
	_, err := LoadKey(Source{Env: "!!!not-base64-and-too-short"})
	if err == nil {
		t.Fatal("expected decode error")
	}
}
