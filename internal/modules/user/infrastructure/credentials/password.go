package credentials

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB   = 19 * 1024
	argonIterations  = 2
	argonParallelism = 1
	saltLength       = 16
	keyLength        = 32
)

type PasswordHasher struct{}

func (PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonParallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemoryKiB, argonIterations, argonParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func (PasswordHasher) Verify(password, encodedHash string) bool {
	parameters, salt, expectedKey, err := parseHash(encodedHash)
	if err != nil {
		return false
	}
	actualKey := argon2.IDKey([]byte(password), salt, parameters.iterations, parameters.memoryKiB, parameters.parallelism, keyLength)
	return subtle.ConstantTimeCompare(actualKey, expectedKey) == 1
}

type hashParameters struct {
	memoryKiB, iterations uint32
	parallelism           uint8
}

func parseHash(encodedHash string) (hashParameters, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return hashParameters{}, nil, nil, errors.New("password hash format is invalid")
	}
	var parameters hashParameters
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parameters.memoryKiB, &parameters.iterations, &parameters.parallelism); err != nil {
		return hashParameters{}, nil, nil, errors.New("password hash parameters are invalid")
	}
	if parameters.memoryKiB != argonMemoryKiB || parameters.iterations != argonIterations || parameters.parallelism != argonParallelism {
		return hashParameters{}, nil, nil, errors.New("password hash parameters are unsafe")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltLength {
		return hashParameters{}, nil, nil, errors.New("password hash salt is invalid")
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) != keyLength {
		return hashParameters{}, nil, nil, errors.New("password hash key is invalid")
	}
	return parameters, salt, key, nil
}
