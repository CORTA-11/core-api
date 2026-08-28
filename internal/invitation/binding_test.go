package invitation

import (
	"bytes"
	"testing"
)

func TestBindingCanonicalizesEmailAndNeverReturnsIt(t *testing.T) {
	binding, err := NewBinding([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := binding.EmailFingerprint("  Person@Example.COM ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := binding.EmailFingerprint("person@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || len(first) != 32 {
		t.Fatalf("fingerprints differ or have wrong length: %x %x", first, second)
	}
}

func TestTokenRoundTripIsCanonicalAndHas256Bits(t *testing.T) {
	binding, err := NewBinding([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := binding.GenerateToken(bytes.NewReader(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, parsedHash, err := binding.ParseToken(token)
	if err != nil || len(parsed) != 32 || !bytes.Equal(hash, parsedHash) {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, _, err := binding.ParseToken(token + "="); err == nil {
		t.Fatal("accepted padded, non-canonical token")
	}
}

func TestBindingRejectsShortSecretAndInvalidEmail(t *testing.T) {
	if _, err := NewBinding([]byte("short")); err == nil {
		t.Fatal("accepted short secret")
	}
	binding, _ := NewBinding([]byte("0123456789abcdef0123456789abcdef"))
	if _, err := binding.EmailFingerprint("bad\x00email"); err == nil {
		t.Fatal("accepted invalid email")
	}
}
