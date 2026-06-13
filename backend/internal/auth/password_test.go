package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordVerifiesPassword(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	if !VerifyPassword(hash, "correct horse") {
		t.Fatal("VerifyPassword = false, want true")
	}
	if VerifyPassword(hash, "wrong horse") {
		t.Fatal("VerifyPassword = true for wrong password, want false")
	}
}

func TestValidatePasswordRejectsInvalidLengths(t *testing.T) {
	for _, password := range []string{"", "short"} {
		if err := ValidatePassword(password); err == nil {
			t.Fatalf("ValidatePassword(%q) error = nil, want error", password)
		}
	}

	longPassword := strings.Repeat("a", MaximumPasswordBytes+1)
	if err := ValidatePassword(longPassword); err == nil {
		t.Fatal("ValidatePassword(longPassword) error = nil, want error")
	}
}

func TestHashPasswordUsesArgon2IDEncodedFormat(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash = %q, want argon2id encoded hash", hash)
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if VerifyPassword("not-a-password-hash", "correct horse") {
		t.Fatal("VerifyPassword = true for malformed hash, want false")
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got, want := NormalizeEmail(" Dev@LOCAL.Soniq "), "dev@local.soniq"; got != want {
		t.Fatalf("NormalizeEmail = %q, want %q", got, want)
	}
}
