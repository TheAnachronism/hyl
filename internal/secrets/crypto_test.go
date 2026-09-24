package secrets

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	cipher, err := New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sealed, err := cipher.EncryptString("intervals-api-key-abc")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if string(sealed) == "intervals-api-key-abc" {
		t.Fatal("ciphertext equals the plaintext")
	}

	opened, err := cipher.DecryptString(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if opened != "intervals-api-key-abc" {
		t.Fatalf("round trip returned %q", opened)
	}

	// The same plaintext must seal differently every time (random nonce).
	other, err := cipher.EncryptString("intervals-api-key-abc")
	if err != nil {
		t.Fatal(err)
	}
	if string(other) == string(sealed) {
		t.Fatal("two seals of the same token are identical")
	}
}

func TestDecryptRejectsTamperingAndWrongKeys(t *testing.T) {
	cipher, err := New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.EncryptString("token")
	if err != nil {
		t.Fatal(err)
	}

	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := cipher.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}

	other, err := New("ffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Decrypt(sealed); err == nil {
		t.Fatal("a token decrypted with the wrong key")
	}

	if _, err := cipher.Decrypt([]byte{1, 2, 3}); err == nil {
		t.Fatal("a short blob decrypted")
	}

	// An empty stored token is "no token", not an error.
	empty, err := cipher.DecryptString(nil)
	if err != nil || empty != "" {
		t.Fatalf("empty token: %q %v", empty, err)
	}
}

func TestNewRejectsShortSecrets(t *testing.T) {
	if _, err := New("too-short"); err == nil {
		t.Fatal("a 9 byte secret was accepted")
	}
}
