package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	platformhttp "github.com/joinname177/dify-agent-api/internal/platform/httputil"
)

type identityKey struct{}

type Identity struct {
	UserID      int64
	PublicID    string
	Email       string
	DisplayName string
	Role        string
	CreatedAt   time.Time
}

type Authenticator interface {
	Authenticate(context.Context, string) (Identity, error)
}

func OptionalMiddleware(authenticator Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}
			scheme, token, ok := strings.Cut(header, " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				writeUnauthenticated(w, r)
				return
			}
			identity, err := authenticator.Authenticate(r.Context(), token)
			if err != nil {
				writeUnauthenticated(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
		})
	}
}

func IdentityFromContext(ctx context.Context) (Identity, error) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	if !ok {
		return Identity{}, errors.New("authenticated identity is missing")
	}
	return identity, nil
}

func writeUnauthenticated(w http.ResponseWriter, r *http.Request) {
	platformhttp.WriteError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
}
