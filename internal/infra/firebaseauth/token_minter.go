package firebaseauth

import (
	"context"
	"fmt"

	"firebase.google.com/go/v4/auth"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

const (
	claimTenantID     = "tenant_id"
	claimEventID      = "event_id"
	claimPeerRole     = "peer_role"
	claimEdgeDeviceID = "edge_device_id"
	claimSessionID    = "session_id"
	claimExpiresAt    = "expires_at"
	claimTokenUse     = "token_use"

	tokenUseSignaling = "signaling"
)

type TokenMinter struct {
	client *auth.Client
}

func NewTokenMinter(client *auth.Client) *TokenMinter {
	return &TokenMinter{client: client}
}

func (m *TokenMinter) Mint(ctx context.Context, access domain.FirestoreAccess) (string, error) {
	claims := map[string]any{
		claimTenantID:  string(access.TenantID),
		claimEventID:   string(access.EventID),
		claimPeerRole:  string(access.Role),
		claimExpiresAt: access.ExpiresAt.Unix(),
		claimTokenUse:  tokenUseSignaling,
	}

	if access.EdgeDeviceID != "" {
		claims[claimEdgeDeviceID] = string(access.EdgeDeviceID)
	}

	if access.SessionID != "" {
		claims[claimSessionID] = string(access.SessionID)
	}

	token, err := m.client.CustomTokenWithClaims(ctx, access.Subject, claims)
	if err != nil {
		return "", fmt.Errorf("create custom token: %w", err)
	}

	return token, nil
}
