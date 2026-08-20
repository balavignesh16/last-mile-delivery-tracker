package auth

import "testing"

func TestHashPassword_VerifyPassword_RoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("hash equals plaintext — password was not actually hashed")
	}
	if err := VerifyPassword(hash, "correct horse battery staple"); err != nil {
		t.Errorf("VerifyPassword() with correct password failed: %v", err)
	}
}

func TestVerifyPassword_RejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("the-real-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if err := VerifyPassword(hash, "a-different-password"); err == nil {
		t.Error("expected VerifyPassword() to reject a wrong password, got nil error")
	}
}

func TestHashPassword_SaltsEachCallDifferently(t *testing.T) {
	hash1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	hash2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if hash1 == hash2 {
		t.Error("two hashes of the same password are identical — bcrypt should salt each call")
	}
	// Both must still verify correctly despite differing.
	if err := VerifyPassword(hash1, "same-password"); err != nil {
		t.Errorf("hash1 failed to verify: %v", err)
	}
	if err := VerifyPassword(hash2, "same-password"); err != nil {
		t.Errorf("hash2 failed to verify: %v", err)
	}
}
