package hash

import "testing"

func TestPassword(t *testing.T) {
	hashed, err := Password("secret123")
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	if hashed == "" || hashed == "secret123" {
		t.Fatalf("unexpected hash: %q", hashed)
	}

	if !CheckPassword(hashed, "secret123") {
		t.Fatal("expected correct password to match")
	}
	if CheckPassword(hashed, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}
