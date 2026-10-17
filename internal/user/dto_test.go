package user

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToPublicDoesNotExposePasswordHash(t *testing.T) {
	u := validTestUser()
	u.PasswordHash = "secret-hash"
	u.TokenVersion = 42
	data, err := json.Marshal(u.ToPublic())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	result := string(data)
	if strings.Contains(result, "secret-hash") {
		t.Fatal("public response exposes password hash")
	}
	if strings.Contains(result, "password_hash") {
		t.Fatal("public response contains password_hash field")
	}
	if strings.Contains(result, "deleted_at") {
		t.Fatal("public response contains deleted_at field")
	}
	if strings.Contains(result, "token_version") {
		t.Fatal("public response contains token_version field")
	}
	if !strings.Contains(result, `"nickname":"moonflux"`) {
		t.Fatalf("public response missing nickname: %s", result)
	}
}
