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

func TestMintUnderTheEmulatorCarriesTheAccessAsCustomClaims(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "127.0.0.1:1")

	ctx := context.Background()

	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: "demo-tolo"})
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	client, err := app.Auth(ctx)
	if err != nil {
		t.Fatalf("Auth() error = %v", err)
	}

	expiresAt := time.Date(2026, time.October, 14, 12, 5, 0, 0, time.UTC)

	access, err := domain.NewFirestoreAccess("user-1", "0123456789abcdef", "fedcba9876543210", domain.PeerRoleConsole, expiresAt)
	if err != nil {
		t.Fatalf("NewFirestoreAccess() error = %v", err)
	}

	token, err := firebaseauth.NewTokenMinter(client).Mint(ctx, access)
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
		UID    string `json:"uid"`
		Claims struct {
			TenantID  string `json:"tenant_id"`
			EventID   string `json:"event_id"`
			PeerRole  string `json:"peer_role"`
			ExpiresAt int64  `json:"expires_at"`
			TokenUse  string `json:"token_use"`
		} `json:"claims"`
	}

	decodeJWTPart(t, parts[1], &payload)

	if got, want := payload.UID, "user-1"; got != want {
		t.Errorf("uid = %q, want %q", got, want)
	}

	if got, want := payload.Claims.TenantID, "0123456789abcdef"; got != want {
		t.Errorf("tenant_id = %q, want %q", got, want)
	}

	if got, want := payload.Claims.EventID, "fedcba9876543210"; got != want {
		t.Errorf("event_id = %q, want %q", got, want)
	}

	if got, want := payload.Claims.PeerRole, "console"; got != want {
		t.Errorf("peer_role = %q, want %q", got, want)
	}

	if got, want := payload.Claims.ExpiresAt, expiresAt.Unix(); got != want {
		t.Errorf("expires_at = %d, want %d", got, want)
	}

	if got, want := payload.Claims.TokenUse, "signaling"; got != want {
		t.Errorf("token_use = %q, want %q", got, want)
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
