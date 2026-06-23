package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordPHCFormat(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash %q does not have $argon2id$ prefix", hash)
	}
	if strings.Contains(hash, "correct horse battery staple") {
		t.Error("plaintext password leaked into the hash")
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret-passphrase")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword(hash, "s3cret-passphrase")
	if err != nil {
		t.Fatalf("VerifyPassword (correct): %v", err)
	}
	if !ok {
		t.Error("VerifyPassword returned false for the correct password")
	}
	bad, err := VerifyPassword(hash, "wrong-password")
	if err != nil {
		t.Fatalf("VerifyPassword (wrong): %v", err)
	}
	if bad {
		t.Error("VerifyPassword returned true for a wrong password")
	}
}
