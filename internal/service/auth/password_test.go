package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordVerifiesTheSamePasswordOnly(t *testing.T) {
	encoded, err := HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash %q does not carry its cost parameters", encoded)
	}

	ok, err := VerifyPassword("a-real-password", encoded)
	if err != nil || !ok {
		t.Fatalf("matching password: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("other-password", encoded)
	if err != nil || ok {
		t.Fatalf("wrong password: ok=%v err=%v", ok, err)
	}
}

func TestVerifyPasswordRejectsAMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("a-real-password", "not-a-phc-string"); err == nil {
		t.Fatal("expected a malformed hash to fail closed")
	}
}
