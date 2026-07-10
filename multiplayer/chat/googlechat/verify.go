package googlechat

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/idtoken"
)

// IDTokenVerifier verifies Google Chat bearer ID tokens for HTTP delivery.
type IDTokenVerifier struct {
	Audience string
}

// Verify validates the Authorization bearer token against the configured audience.
func (v IDTokenVerifier) Verify(ctx context.Context, r *http.Request) error {
	audience := strings.TrimSpace(v.Audience)
	if audience == "" {
		return fmt.Errorf("googlechat: verifier audience is empty")
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return fmt.Errorf("googlechat: missing bearer token")
	}
	if _, err := idtoken.Validate(ctx, token, audience); err != nil {
		return fmt.Errorf("googlechat: validate bearer token: %w", err)
	}
	return nil
}

func bearerToken(header string) string {
	const prefix = "bearer "
	header = strings.TrimSpace(header)
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
