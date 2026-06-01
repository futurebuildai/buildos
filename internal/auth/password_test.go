package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPassword_RoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash prefix: %q", hash)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Fatalf("VerifyPassword on correct password: %v", err)
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	err = VerifyPassword("hunter3", hash)
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("err = %v, want ErrPasswordMismatch", err)
	}
}

func TestHashPassword_SaltIsRandom(t *testing.T) {
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword #1: %v", err)
	}
	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword #2: %v", err)
	}
	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical — salt is not random")
	}
	// Both must still verify.
	if err := VerifyPassword("same-password", h1); err != nil {
		t.Errorf("verify h1: %v", err)
	}
	if err := VerifyPassword("same-password", h2); err != nil {
		t.Errorf("verify h2: %v", err)
	}
}

func TestHashPassword_EmptyRejected(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error hashing empty password")
	}
}

func TestVerifyPassword_MalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=19456,t=2,p=1$only-one-field",
		"$bcrypt$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA",      // wrong algo
		"$argon2id$v=999$m=19456,t=2,p=1$c2FsdA$aGFzaA",   // wrong version
		"$argon2id$v=19$bad-params$c2FsdA$aGFzaA",         // bad params
		"$argon2id$v=19$m=19456,t=2,p=1$!!!notb64$aGFzaA", // bad salt b64
		"$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$!!!notb64", // bad hash b64
	}
	for _, c := range cases {
		if err := VerifyPassword("whatever", c); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("VerifyPassword(%q) err = %v, want ErrInvalidHash", c, err)
		}
	}
}
