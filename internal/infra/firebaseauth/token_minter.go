package firebaseauth

import (
	"context"
	"fmt"

	"firebase.google.com/go/v4/auth"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

const (
	claimTenantID  = "tenant_id"
	claimEventID   = "event_id"
	claimPeerRole  = "peer_role"
	claimExpiresAt = "expires_at"
)

type TokenMinter struct {
	client *auth.Client
}

func NewTokenMinter(client *auth.Client) *TokenMinter {
	return &TokenMinter{client: client}
}

func (m *TokenMinter) Mint(ctx context.Context, access domain.FirestoreAccess) (string, error) {
	token, err := m.client.CustomTokenWithClaims(ctx, access.Subject, map[string]any{
		claimTenantID:  string(access.TenantID),
		claimEventID:   string(access.EventID),
		claimPeerRole:  string(access.Role),
		claimExpiresAt: access.ExpiresAt.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("create custom token: %w", err)
	}

	return token, nil
}
