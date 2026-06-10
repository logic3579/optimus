package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestGenerate_ProducesBase64Of32Bytes(t *testing.T) {
	var buf bytes.Buffer
	if err := generate(&buf); err != nil {
		t.Fatalf("generate: %v", err)
	}
	out := buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", out)
	}
	body := bytes.TrimRight(out, "\n")
	raw, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		t.Fatalf("output not valid base64: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("decoded len = %d, want 32", len(raw))
	}
}

func TestGenerate_TwoCallsDiffer(t *testing.T) {
	var a, b bytes.Buffer
	_ = generate(&a)
	_ = generate(&b)
	if a.String() == b.String() {
		t.Error("two generates produced identical output — randomness broken")
	}
}
