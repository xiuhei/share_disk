package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters
const (
	Argon2Time    = 3
	Argon2Memory  = 64 * 1024 // 64 MB
	Argon2Threads = 4
	Argon2KeyLen  = 32
	Argon2SaltLen = 16
)

// HashPassword hashes a password using Argon2id
func HashPassword(password string) (string, error) {
	// Generate random salt
	salt := make([]byte, Argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Hash password with Argon2id
	hash := argon2.IDKey([]byte(password), salt, Argon2Time, Argon2Memory, Argon2Threads, Argon2KeyLen)

	// Encode to base64
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	// Format: $argon2id$v=19$m=65536,t=3,p=4$salt$hash
	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, Argon2Memory, Argon2Time, Argon2Threads, saltB64, hashB64)

	return encoded, nil
}

// VerifyPassword verifies a password against a hash
func VerifyPassword(password, encodedHash string) (bool, error) {
	// Parse the encoded hash
	params, salt, hash, err := parseHash(encodedHash)
	if err != nil {
		return false, fmt.Errorf("failed to parse hash: %w", err)
	}

	// Hash the password with the same parameters
	otherHash := argon2.IDKey([]byte(password), salt, params.Time, params.Memory, params.Threads, params.KeyLen)

	// Constant-time comparison
	if subtle.ConstantTimeCompare(hash, otherHash) == 1 {
		return true, nil
	}

	return false, nil
}

type argon2Params struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
}

func parseHash(encodedHash string) (*argon2Params, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return nil, nil, nil, fmt.Errorf("invalid hash format")
	}

	if parts[1] != "argon2id" {
		return nil, nil, nil, fmt.Errorf("unsupported algorithm: %s", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, nil, fmt.Errorf("unsupported version: %d", version)
	}

	params := &argon2Params{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Time, &params.Threads); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid params: %w", err)
	}
	params.KeyLen = Argon2KeyLen

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid salt: %w", err)
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid hash: %w", err)
	}

	return params, salt, hash, nil
}
