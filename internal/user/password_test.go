package user

import (
	"strings"
	"testing"
)

func TestHashPasswordAndVerifyPassword(t *testing.T) {
	password := "  a strong password  "

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if hash == password {
		t.Fatal("HashPassword() returned the original password")
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %q", hash)
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword() rejected the correct password")
	}

	ok, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword() for wrong password returned error: %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword() accepted the wrong password")
	}
}

func TestHashPasswordRejectsEmptyPassword(t *testing.T) {
	_, err := HashPassword("")
	if err == nil {
		t.Fatal("HashPassword() error = nil, want error")
	}
}

func TestVerifyPasswordRejectsInvalidHashes(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{name: "empty", hash: ""},
		{name: "wrong algorithm", hash: "$bcrypt$invalid"},
		{name: "malformed", hash: "$argon2id$v=19$m=invalid,t=2,p=1$salt$key"},
		{name: "excessive memory", hash: "$argon2id$v=19$m=4294967295,t=2,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcA"},
		{name: "invalid salt", hash: "$argon2id$v=19$m=19456,t=2,p=1$!$YWJjZGVmZ2hpamtsbW5vcA"},
		{name: "invalid key", hash: "$argon2id$v=19$m=19456,t=2,p=1$YWJjZGVmZ2hpamtsbW5vcA$!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyPassword("some password", tt.hash)
			if err == nil {
				t.Fatal("VerifyPassword() error = nil, want error")
			}
			if ok {
				t.Fatal("VerifyPassword() accepted an invalid hash")
			}
		})
	}
}
