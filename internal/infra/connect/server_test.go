package connect

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/mock/gomock"

	edgebridgev1 "github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1"
	"github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1/edgebridgev1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application/applicationmock"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/externaltoken"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/connect/authn"
)

const (
	testIssuer      = "https://idp.example.test"
	testAudience    = "backend-api"
	consoleClientID = "console-ui"
	edgeClientID    = "edge-page"
	testTenantID    = "0123456789abcdef"
	testEventID     = "fedcba9876543210"
)

type testIdP struct {
	server *httptest.Server
	status atomic.Int32
	fetch  atomic.Int32

	mu  sync.Mutex
	key *rsa.PrivateKey
	kid string
	now time.Time
}

func newTestIdP(t *testing.T) *testIdP {
	t.Helper()

	idp := &testIdP{
		server: nil,
		status: atomic.Int32{},
		fetch:  atomic.Int32{},
		mu:     sync.Mutex{},
		key:    nil,
		kid:    "",
		now:    time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC),
	}
	idp.status.Store(http.StatusOK)
	idp.rotate(t, "kid-1")

	idp.server = httptest.NewServer(http.HandlerFunc(idp.serveJWKS))
	t.Cleanup(idp.server.Close)

	return idp
}

func (i *testIdP) rotate(t *testing.T, kid string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	i.key = key
	i.kid = kid
}

func (i *testIdP) advance(d time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.now = i.now.Add(d)
}

func (i *testIdP) clock() time.Time {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.now
}

func (i *testIdP) serveJWKS(w http.ResponseWriter, _ *http.Request) {
	i.fetch.Add(1)

	if status := int(i.status.Load()); status != http.StatusOK {
		w.WriteHeader(status)

		return
	}

	i.mu.Lock()
	public := i.key.PublicKey
	kid := i.kid
	i.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
		"kty": "RSA",
		"kid": kid,
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(public.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(public.E)).Bytes()),
	}}})
}

func (i *testIdP) token(t *testing.T, mutate func(claims jwt.MapClaims)) string {
	t.Helper()

	now := i.clock()
	claims := jwt.MapClaims{
		"iss":       testIssuer,
		"aud":       testAudience,
		"sub":       "user-1",
		"client_id": consoleClientID,
		"token_use": externaltoken.TokenUseEventAccess,
		"scope":     "events.read",
		"jti":       "jti-1",
		"tenant_id": testTenantID,
		"event_id":  testEventID,
		"iat":       now.Unix(),
		"nbf":       now.Unix(),
		"exp":       now.Add(5 * time.Minute).Unix(),
	}

	if mutate != nil {
		mutate(claims)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = i.kid

	signed, err := token.SignedString(i.key)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	return signed
}

func newTestRepository(t *testing.T) *applicationmock.MockEdgeRepository {
	t.Helper()

	return applicationmock.NewMockEdgeRepository(gomock.NewController(t))
}

func newTestMinter(t *testing.T) *applicationmock.MockFirestoreTokenMinter {
	t.Helper()

	return applicationmock.NewMockFirestoreTokenMinter(gomock.NewController(t))
}

func newSignalingService(repo application.EdgeRepository) *application.SignalingService {
	return application.NewSignalingService(repo, nil)
}

func newTestServer(t *testing.T, repo application.EdgeRepository, idp *testIdP) *httptest.Server {
	t.Helper()

	return newTestServerWith(t, newSignalingService(repo), idp)
}

func newTestServerWith(t *testing.T, service *application.SignalingService, idp *testIdP) *httptest.Server {
	t.Helper()

	keys, err := externaltoken.NewKeySet(externaltoken.KeySetConfig{
		URL:             idp.server.URL + "/jwks",
		HTTPClient:      idp.server.Client(),
		CacheTTL:        0,
		RefreshCooldown: 0,
		FailureCooldown: 0,
		FetchTimeout:    0,
		RetryBackoff:    []time.Duration{},
		MaxDocumentSize: 0,
		Clock:           idp.clock,
	})
	if err != nil {
		t.Fatalf("NewKeySet() error = %v", err)
	}

	verifier, err := externaltoken.NewVerifier(externaltoken.Config{
		Issuer:     testIssuer,
		Audience:   testAudience,
		Algorithms: nil,
		Keys:       keys,
		Leeway:     0,
		Clock:      idp.clock,
	})
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}

	authenticator, err := authn.New(verifier, map[string]domain.PeerRole{consoleClientID: domain.PeerRoleConsole, edgeClientID: domain.PeerRoleEdge})
	if err != nil {
		t.Fatalf("authn.New() error = %v", err)
	}

	routes, err := Routes(service, authenticator)
	if err != nil {
		t.Fatalf("Routes() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func newRequest(token, edgeID string) *connectrpc.Request[edgebridgev1.RequestConnectionRequest] {
	req := connectrpc.NewRequest(&edgebridgev1.RequestConnectionRequest{EdgeDeviceId: edgeID})
	if token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}

	return req
}

func requestConnection(t *testing.T, server *httptest.Server, token string) (*connectrpc.Response[edgebridgev1.RequestConnectionResponse], error) {
	t.Helper()

	client := edgebridgev1connect.NewSignalingServiceClient(server.Client(), server.URL)

	return client.RequestConnection(context.Background(), newRequest(token, "edge-1"))
}

func TestRoutesServeRequestConnectionForConsoleToken(t *testing.T) {
	t.Parallel()

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), domain.TenantID(testTenantID), domain.EventID(testEventID), domain.EdgeID("edge-1")).Return(domain.SessionID("sess-1"), nil)

	idp := newTestIdP(t)
	server := newTestServer(t, repo, idp)

	res, err := requestConnection(t, server, idp.token(t, nil))
	if err != nil {
		t.Fatalf("RequestConnection() error = %v", err)
	}

	if got, want := res.Msg.GetSessionId(), "sess-1"; got != want {
		t.Errorf("SessionId = %q, want %q", got, want)
	}
}

func TestRoutesRejectTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(claims jwt.MapClaims)
		want   connectrpc.Code
	}{
		{name: "expired", mutate: func(c jwt.MapClaims) { c["exp"] = c["iat"].(int64) - 60 }, want: connectrpc.CodeUnauthenticated},
		{name: "not yet valid", mutate: func(c jwt.MapClaims) { c["nbf"] = c["iat"].(int64) + 60 }, want: connectrpc.CodeUnauthenticated},
		{name: "wrong issuer", mutate: func(c jwt.MapClaims) { c["iss"] = "https://other.example.test" }, want: connectrpc.CodeUnauthenticated},
		{name: "wrong audience", mutate: func(c jwt.MapClaims) { c["aud"] = "other-api" }, want: connectrpc.CodeUnauthenticated},
		{name: "wrong token use", mutate: func(c jwt.MapClaims) { c["token_use"] = "tenant_access" }, want: connectrpc.CodeUnauthenticated},
		{name: "missing tenant id", mutate: func(c jwt.MapClaims) { delete(c, "tenant_id") }, want: connectrpc.CodeUnauthenticated},
		{name: "missing event id", mutate: func(c jwt.MapClaims) { delete(c, "event_id") }, want: connectrpc.CodeUnauthenticated},
		{name: "key bound", mutate: func(c jwt.MapClaims) { c["cnf"] = map[string]any{"jkt": "thumbprint"} }, want: connectrpc.CodeUnauthenticated},
		{name: "unknown client", mutate: func(c jwt.MapClaims) { c["client_id"] = "other-client" }, want: connectrpc.CodePermissionDenied},
		{name: "missing scope", mutate: func(c jwt.MapClaims) { c["scope"] = "events.manage" }, want: connectrpc.CodePermissionDenied},
		{name: "edge role", mutate: func(c jwt.MapClaims) { c["client_id"] = edgeClientID }, want: connectrpc.CodePermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			server := newTestServer(t, newTestRepository(t), idp)

			_, err := requestConnection(t, server, idp.token(t, tt.mutate))
			if got := connectrpc.CodeOf(err); got != tt.want {
				t.Fatalf("RequestConnection() error code = %v, want %v (%v)", got, tt.want, err)
			}

			var connectErr *connectrpc.Error
			if errors.As(err, &connectErr) {
				for _, secret := range []string{testTenantID, testEventID, consoleClientID, edgeClientID, "other-client"} {
					if strings.Contains(connectErr.Message(), secret) {
						t.Errorf("error message %q leaks %q", connectErr.Message(), secret)
					}
				}
			}
		})
	}
}

func TestRoutesAcceptEventsReportScope(t *testing.T) {
	t.Parallel()

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.SessionID("sess-1"), nil)

	idp := newTestIdP(t)
	server := newTestServer(t, repo, idp)

	if _, err := requestConnection(t, server, idp.token(t, func(c jwt.MapClaims) { c["scope"] = "events.report" })); err != nil {
		t.Fatalf("RequestConnection() error = %v", err)
	}
}

func TestRoutesRejectMissingTokenBeforeDecoding(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	server := newTestServer(t, newTestRepository(t), idp)

	res, err := server.Client().Post(server.URL+edgebridgev1connect.SignalingServiceRequestConnectionProcedure, "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}

	defer res.Body.Close()

	if got, want := res.StatusCode, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	var decoded struct {
		Code string `json:"code"`
	}

	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", body, err)
	}

	if got, want := decoded.Code, "unauthenticated"; got != want {
		t.Errorf("code = %q, want %q", got, want)
	}

	if got := idp.fetch.Load(); got != 0 {
		t.Errorf("JWKS fetched %d times, want 0", got)
	}
}

func TestRoutesRefetchJWKSForUnknownKid(t *testing.T) {
	t.Parallel()

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.SessionID("sess-1"), nil).Times(2)

	idp := newTestIdP(t)
	server := newTestServer(t, repo, idp)

	if _, err := requestConnection(t, server, idp.token(t, nil)); err != nil {
		t.Fatalf("RequestConnection() with the first key error = %v", err)
	}

	idp.rotate(t, "kid-2")

	_, err := requestConnection(t, server, idp.token(t, nil))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnavailable; got != want {
		t.Fatalf("RequestConnection() within the refresh cooldown error code = %v, want %v (%v)", got, want, err)
	}

	if got, want := idp.fetch.Load(), int32(1); got != want {
		t.Errorf("JWKS fetched %d times within the cooldown, want %d", got, want)
	}

	idp.advance(externaltoken.DefaultRefreshCooldown)

	if _, err := requestConnection(t, server, idp.token(t, nil)); err != nil {
		t.Fatalf("RequestConnection() after the cooldown error = %v", err)
	}

	if got, want := idp.fetch.Load(), int32(2); got != want {
		t.Errorf("JWKS fetched %d times after the cooldown, want %d", got, want)
	}
}

func TestRoutesRejectStaleKidAfterRefetch(t *testing.T) {
	t.Parallel()

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.SessionID("sess-1"), nil)

	idp := newTestIdP(t)
	server := newTestServer(t, repo, idp)
	stale := idp.token(t, nil)

	idp.rotate(t, "kid-2")

	if _, err := requestConnection(t, server, idp.token(t, nil)); err != nil {
		t.Fatalf("RequestConnection() with the current key error = %v", err)
	}

	idp.advance(externaltoken.DefaultRefreshCooldown)

	_, err := requestConnection(t, server, stale)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("RequestConnection() with a stale kid error code = %v, want %v (%v)", got, want, err)
	}
}

func TestRoutesAnswerUnavailableWhenJWKSFails(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	idp.status.Store(http.StatusInternalServerError)
	server := newTestServer(t, newTestRepository(t), idp)

	_, err := requestConnection(t, server, idp.token(t, nil))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnavailable; got != want {
		t.Fatalf("RequestConnection() error code = %v, want %v (%v)", got, want, err)
	}
}

func TestRoutesRejectEmptyEdgeID(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	server := newTestServer(t, newTestRepository(t), idp)
	client := edgebridgev1connect.NewSignalingServiceClient(server.Client(), server.URL)

	_, err := client.RequestConnection(context.Background(), newRequest(idp.token(t, nil), ""))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInvalidArgument; got != want {
		t.Fatalf("RequestConnection() error code = %v, want %v", got, want)
	}
}

func TestServiceWithoutPrincipalIsUnauthenticated(t *testing.T) {
	t.Parallel()

	service := NewService(newSignalingService(newTestRepository(t)))

	_, err := service.RequestConnection(context.Background(), newRequest("", "edge-1"))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("RequestConnection() error code = %v, want %v", got, want)
	}

	_, err = service.IssueFirestoreToken(context.Background(), connectrpc.NewRequest(&edgebridgev1.IssueFirestoreTokenRequest{}))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("IssueFirestoreToken() error code = %v, want %v", got, want)
	}
}

func issueFirestoreToken(t *testing.T, server *httptest.Server, token string) (*connectrpc.Response[edgebridgev1.IssueFirestoreTokenResponse], error) {
	t.Helper()

	client := edgebridgev1connect.NewSignalingServiceClient(server.Client(), server.URL)

	req := connectrpc.NewRequest(&edgebridgev1.IssueFirestoreTokenRequest{})
	if token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}

	return client.IssueFirestoreToken(context.Background(), req)
}

func TestRoutesIssueFirestoreTokenForEachRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		clientID string
		role     domain.PeerRole
	}{
		{name: "edge", clientID: edgeClientID, role: domain.PeerRoleEdge},
		{name: "console", clientID: consoleClientID, role: domain.PeerRoleConsole},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			wantExpiresAt := idp.clock().Add(5 * time.Minute)

			var minted domain.FirestoreAccess

			minter := newTestMinter(t)
			minter.EXPECT().Mint(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, access domain.FirestoreAccess) (string, error) {
				minted = access

				return "custom-token", nil
			})

			server := newTestServerWith(t, application.NewSignalingService(newTestRepository(t), minter), idp)

			res, err := issueFirestoreToken(t, server, idp.token(t, func(c jwt.MapClaims) { c["client_id"] = tt.clientID; c["sub"] = "user-" + tt.name }))
			if err != nil {
				t.Fatalf("IssueFirestoreToken() error = %v", err)
			}

			if got, want := res.Msg.GetFirebaseCustomToken(), "custom-token"; got != want {
				t.Errorf("FirebaseCustomToken = %q, want %q", got, want)
			}

			if got := res.Msg.GetExpiresAt().AsTime(); !got.Equal(wantExpiresAt) {
				t.Errorf("ExpiresAt = %v, want %v", got, wantExpiresAt)
			}

			want := domain.FirestoreAccess{Subject: "user-" + tt.name, TenantID: testTenantID, EventID: testEventID, Role: tt.role, ExpiresAt: wantExpiresAt}
			if minted.Subject != want.Subject || minted.TenantID != want.TenantID || minted.EventID != want.EventID || minted.Role != want.Role || !minted.ExpiresAt.Equal(want.ExpiresAt) {
				t.Errorf("minted access = %+v, want %+v", minted, want)
			}
		})
	}
}

func TestRoutesRejectIssueFirestoreTokenWithoutToken(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	server := newTestServerWith(t, application.NewSignalingService(newTestRepository(t), newTestMinter(t)), idp)

	_, err := issueFirestoreToken(t, server, "")
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("IssueFirestoreToken() error code = %v, want %v (%v)", got, want, err)
	}
}

func TestServiceMinterFailureIsInternal(t *testing.T) {
	captureLog(t)

	minter := newTestMinter(t)
	minter.EXPECT().Mint(gomock.Any(), gomock.Any()).Return("", errors.New("signBlob denied"))
	service := NewService(application.NewSignalingService(newTestRepository(t), minter))

	ctx := authn.NewContext(context.Background(), authn.Principal{Subject: "user-1", ClientID: edgeClientID, Role: domain.PeerRoleEdge, TenantID: testTenantID, EventID: testEventID, ExpiresAt: time.Now()})

	_, err := service.IssueFirestoreToken(ctx, connectrpc.NewRequest(&edgebridgev1.IssueFirestoreTokenRequest{}))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInternal; got != want {
		t.Errorf("IssueFirestoreToken() error code = %v, want %v", got, want)
	}

	if strings.Contains(err.Error(), "signBlob") {
		t.Errorf("error = %q, want it to omit the underlying failure", err)
	}
}

func TestServiceRepositoryFailureIsInternal(t *testing.T) {
	captureLog(t)

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), domain.TenantID(testTenantID), domain.EventID(testEventID), domain.EdgeID("edge-1")).Return(domain.SessionID(""), errors.New("firestore unavailable"))
	service := NewService(newSignalingService(repo))

	ctx := authn.NewContext(context.Background(), authn.Principal{ClientID: consoleClientID, Role: domain.PeerRoleConsole, TenantID: testTenantID, EventID: testEventID})

	_, err := service.RequestConnection(ctx, newRequest("", "edge-1"))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInternal; got != want {
		t.Errorf("RequestConnection() error code = %v, want %v", got, want)
	}
}
