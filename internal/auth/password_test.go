package auth

import (
	"crypto/sha256"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	const password = "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == password {
		t.Fatal("hash equals the plaintext")
	}

	ok, err := VerifyPassword(hash, password)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("the correct password did not verify")
	}

	ok, err = VerifyPassword(hash, password+"x")
	if err != nil {
		t.Fatalf("VerifyPassword (wrong): %v", err)
	}
	if ok {
		t.Fatal("a wrong password verified")
	}

	// The same password must produce a different hash every time (random salt).
	other, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if other == hash {
		t.Fatal("two hashes of the same password are identical")
	}
	ok, err = VerifyPassword(other, password)
	if err != nil || !ok {
		t.Fatalf("second hash did not verify: %v", err)
	}
}

func TestVerifyPasswordRejectsTamperedHashes(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"empty":            "",
		"truncated":        hash[:len(hash)-4],
		"wrong prefix":     "$argon2i$v=19$m=47104,t=1,p=1$c2FsdA$a2V5",
		"bad params":       "$argon2id$v=19$m=0,t=0,p=0$c2FsdA$a2V5",
		"wrong version":    "$argon2id$v=13$m=47104,t=1,p=1$c2FsdA$a2V5",
		"bad base64 salt":  "$argon2id$v=19$m=47104,t=1,p=1$!!!!$a2V5",
		"missing segments": "$argon2id$v=19$m=47104,t=1,p=1",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := VerifyPassword(encoded, "whatever"); err == nil {
				t.Fatalf("tampered hash %q was accepted", encoded)
			}
		})
	}

	// A tampered key must never verify: either it is rejected outright or the
	// comparison fails.
	keyed := hash[:len(hash)-1]
	if hash[len(hash)-1] == 'A' {
		keyed += "B"
	} else {
		keyed += "A"
	}
	if ok, err := VerifyPassword(keyed, "correct horse battery staple"); err == nil && ok {
		t.Fatal("a tampered key verified")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	if err := ValidatePasswordStrength("short"); err == nil {
		t.Fatal("a 5 character password was accepted")
	}
	if err := ValidatePasswordStrength("0123456789"); err != nil {
		t.Fatalf("a 10 character password was rejected: %v", err)
	}
	long := make([]byte, maxPasswordLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidatePasswordStrength(string(long)); err == nil {
		t.Fatal("an over-long password was accepted")
	}
}

func TestSha256SumMatchesTokenHash(t *testing.T) {
	token, sum, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) == 0 {
		t.Fatal("empty token")
	}
	if sum != sha256.Sum256([]byte(token)) {
		t.Fatal("token hash mismatch")
	}
}
