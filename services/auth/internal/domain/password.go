package domain

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Memory      = 64 * 1024 // 64 MiB
	argon2Iterations  = 3
	argon2Parallelism = 2
	argon2SaltLen     = 16
	argon2KeyLen      = 32
)

// HashPassword returns a PHC-format Argon2id hash of password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2Memory, argon2Parallelism, argon2KeyLen)
	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2Memory,
		argon2Iterations,
		argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword compares password against a PHC-format Argon2id hash.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var memory, iters uint32
	var parallelism uint8
	for _, param := range strings.Split(parts[3], ",") {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) != 2 {
			continue
		}
		val, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return false
		}
		switch kv[0] {
		case "m":
			memory = uint32(val)
		case "t":
			iters = uint32(val)
		case "p":
			parallelism = uint8(val)
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	storedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	newHash := argon2.IDKey([]byte(password), salt, iters, memory, parallelism, uint32(len(storedHash)))
	return subtle.ConstantTimeCompare(newHash, storedHash) == 1
}

// CheckPasswordPolicy returns ErrPasswordTooWeak if the policy is not met.
func CheckPasswordPolicy(password string, minLen, maxLen int) error {
	if len(password) < minLen || len(password) > maxLen {
		return ErrPasswordTooWeak
	}
	return nil
}

// dummySalt is used for timing-safe login when a user does not exist.
var dummySalt = []byte("teamboard-dummy-salt-value-1234")
