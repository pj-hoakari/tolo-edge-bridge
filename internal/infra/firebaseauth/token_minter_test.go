package firebaseauth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	firebase "firebase.google.com/go/v4"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/firebaseauth"
)

const (
	testTenantID = "0123456789abcdef"
	testEventID  = "fedcba9876543210"
	testEdgeID   = "0011223344556677"
)

type customClaims struct {
	TenantID     string `json:"tenant_id"`
	EventID      string `json:"event_id"`
	PeerRole     string `json:"peer_role"`
	EdgeDeviceID string `json:"edge_device_id"`
	SessionID    string `json:"session_id"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenUse     string `json:"token_use"`
}

func TestMintUnderTheEmulatorCarriesTheAccessAsCustomClaims(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "127.0.0.1:1")

	expiresAt := time.Date(2026, time.October, 14, 12, 5, 0, 0, time.UTC)

	tests := []struct {
		name   string
		access domain.FirestoreAccess
		want   customClaims
	}{
		{
			name: "management console",
			access: domain.FirestoreAccess{
				Subject:   "user-1",
				TenantID:  testTenantID,
				EventID:   testEventID,
				Role:      domain.PeerRoleConsole,
				ExpiresAt: expiresAt,
			},
			want: customClaims{TenantID: testTenantID, EventID: testEventID, PeerRole: "console", ExpiresAt: expiresAt.Unix(), TokenUse: "signaling"},
		},
		{
			name: "edge device",
			access: domain.FirestoreAccess{
				Subject:      "user-1",
				TenantID:     testTenantID,
				EventID:      testEventID,
				Role:         domain.PeerRoleEdge,
				EdgeDeviceID: testEdgeID,
				ExpiresAt:    expiresAt,
			},
			want: customClaims{TenantID: testTenantID, EventID: testEventID, PeerRole: "edge", EdgeDeviceID: testEdgeID, ExpiresAt: expiresAt.Unix(), TokenUse: "signaling"},
		},
		{
			name: "connection",
			access: domain.FirestoreAccess{
				Subject:      "user-1",
				TenantID:     testTenantID,
				EventID:      testEventID,
				Role:         domain.PeerRoleConsole,
				EdgeDeviceID: testEdgeID,
				SessionID:    "sess-1",
				ExpiresAt:    expiresAt,
			},
			want: customClaims{TenantID: testTenantID, EventID: testEventID, PeerRole: "console", EdgeDeviceID: testEdgeID, SessionID: "sess-1", ExpiresAt: expiresAt.Unix(), TokenUse: "signaling"},
		},
	}

	ctx := context.Background()

	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: "demo-tolo"})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	client, err := app.Auth(ctx)
	if err != nil {
		t.Fatalf("Auth() error = %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := firebaseauth.NewTokenMinter(client).Mint(ctx, tt.access)
			if err != nil {
				t.Fatalf("Mint() error = %v", err)
			}

			parts := strings.Split(token, ".")
			if len(parts) != 3 {
				t.Fatalf("token has %d parts, want 3", len(parts))
			}

			var header struct {
				Alg string `json:"alg"`
			}

			decodeJWTPart(t, parts[0], &header)

			if got, want := header.Alg, "none"; got != want {
				t.Errorf("alg = %q, want %q (the emulator signer emits unsigned tokens)", got, want)
			}

			if parts[2] != "" {
				t.Errorf("signature = %q, want empty under the emulator", parts[2])
			}

			var payload struct {
				UID    string       `json:"uid"`
				Claims customClaims `json:"claims"`
			}

			decodeJWTPart(t, parts[1], &payload)

			if got, want := payload.UID, "user-1"; got != want {
				t.Errorf("uid = %q, want %q", got, want)
			}

			if payload.Claims != tt.want {
				t.Errorf("claims = %+v, want %+v", payload.Claims, tt.want)
			}
		})
	}
}

func decodeJWTPart(t *testing.T, part string, into any) {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatalf("decode %q: %v", part, err)
	}

	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
}
