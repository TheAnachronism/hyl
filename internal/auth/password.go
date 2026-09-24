package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters from the OWASP password storage cheat sheet
// (m=47104 KiB, t=1, p=1).
const (
	argonTime    = 1
	argonMemory  = 47104
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// Password policy.
const (
	minPasswordLen = 10
	maxPasswordLen = 200
)

// ErrInvalidHash is returned when a stored hash cannot be parsed.
var ErrInvalidHash = errors.New("invalid password hash")

// HashPassword returns a PHC-formatted argon2id hash of password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches the encoded PHC hash. The
// parameters are read from the hash, so hashes written with older settings keep
// verifying after the constants above change.
func VerifyPassword(encoded, password string) (bool, error) {
	params, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	// The key length is always argonKeyLen, never the stored key's length: a
	// truncated or padded hash must fail the comparison rather than shorten the
	// derived key.
	other := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, argonKeyLen)
	return subtle.ConstantTimeCompare(key, other) == 1, nil
}

// ValidatePasswordStrength enforces the password policy.
func ValidatePasswordStrength(password string) error {
	n := utf8.RuneCountInString(password)
	if n < minPasswordLen {
		return fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}
	if n > maxPasswordLen {
		return fmt.Errorf("password must be at most %d characters", maxPasswordLen)
	}
	return nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	var p argonParams
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, nil, nil, ErrInvalidHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return p, nil, nil, ErrInvalidHash
	}
	if p.memory == 0 || p.time == 0 || p.threads == 0 {
		return p, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return p, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) != argonKeyLen {
		return p, nil, nil, ErrInvalidHash
	}
	return p, salt, key, nil
}

// randomToken returns a 32-byte URL-safe token and its sha256, the only form
// that is ever stored.
func randomToken() (token string, sum [32]byte, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", sum, err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	sum = sha256Sum(token)
	return token, sum, nil
}

// sha256Sum is the only form of a session or email token that is ever stored.
func sha256Sum(token string) [32]byte { return sha256.Sum256([]byte(token)) }
