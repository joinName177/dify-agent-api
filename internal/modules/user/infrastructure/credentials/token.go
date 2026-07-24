package credentials

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/joinname177/dify-agent-api/internal/modules/user/application/ports"
)

const tokenBytes = 32

type TokenIssuer struct{}

func (TokenIssuer) Issue() (ports.Token, error) {
	random := make([]byte, tokenBytes)
	if _, err := rand.Read(random); err != nil {
		return ports.Token{}, fmt.Errorf("generate token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(random)
	return ports.Token{Raw: raw, Hash: hashToken(raw)}, nil
}

func (TokenIssuer) Hash(raw string) []byte { return hashToken(raw) }

func hashToken(raw string) []byte {
	digest := sha256.Sum256([]byte(raw))
	return digest[:]
}
