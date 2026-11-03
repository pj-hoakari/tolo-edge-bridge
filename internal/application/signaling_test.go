package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application/applicationmock"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

const (
	testTenantID = "0123456789abcdef"
	testEventID  = "fedcba9876543210"
	testEdgeID   = "0011223344556677"
)

func newService(t *testing.T) (*application.SignalingService, *applicationmock.MockEdgeRepository) {
	t.Helper()

	repo := applicationmock.NewMockEdgeRepository(gomock.NewController(t))

	return application.NewSignalingService(repo, nil), repo
}

func newTokenService(t *testing.T) (*application.SignalingService, *applicationmock.MockEdgeRepository, *applicationmock.MockFirestoreTokenMinter) {
	t.Helper()

	repo := applicationmock.NewMockEdgeRepository(gomock.NewController(t))
	minter := applicationmock.NewMockFirestoreTokenMinter(gomock.NewController(t))

	return application.NewSignalingService(repo, minter), repo, minter
}

func validTokenInput() application.IssueFirestoreTokenInput {
	return application.IssueFirestoreTokenInput{
		Subject:      "user-1",
		TenantID:     testTenantID,
		EventID:      testEventID,
		Role:         domain.PeerRoleEdge,
		EdgeDeviceID: testEdgeID,
		ExpiresAt:    time.Date(2026, time.October, 14, 12, 5, 0, 750_000_000, time.FixedZone("JST", 9*60*60)),
	}
}

func validConnectionInput() application.IssueFirestoreTokenInput {
	input := validTokenInput()
	input.Role = domain.PeerRoleConsole
	input.Connection = true
	input.SessionID = "sess-1"

	return input
}

func TestIssueFirestoreToken(t *testing.T) {
	t.Parallel()

	service, _, minter := newTokenService(t)
	wantAccess := domain.FirestoreAccess{
		Subject:      "user-1",
		TenantID:     testTenantID,
		EventID:      testEventID,
		Role:         domain.PeerRoleEdge,
		EdgeDeviceID: testEdgeID,
		ExpiresAt:    time.Date(2026, time.October, 14, 3, 5, 0, 0, time.UTC),
	}
	minter.EXPECT().Mint(gomock.Any(), wantAccess).Return("custom-token", nil)

	token, err := service.IssueFirestoreToken(context.Background(), validTokenInput())
	if err != nil {
		t.Fatalf("IssueFirestoreToken() error = %v", err)
	}

	if want := "custom-token"; token.Token != want {
		t.Errorf("Token = %q, want %q", token.Token, want)
	}

	if !token.ExpiresAt.Equal(wantAccess.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", token.ExpiresAt, wantAccess.ExpiresAt)
	}
}

func TestIssueFirestoreTokenForConnectionClient(t *testing.T) {
	t.Parallel()

	service, repo, minter := newTokenService(t)
	repo.EXPECT().SessionExists(gomock.Any(), domain.TenantID(testTenantID), domain.EventID(testEventID), domain.EdgeID(testEdgeID), domain.SessionID("sess-1")).Return(true, nil)
	minter.EXPECT().Mint(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, access domain.FirestoreAccess) (string, error) {
		if got, want := access.EdgeDeviceID, domain.EdgeID(testEdgeID); got != want {
			t.Errorf("EdgeDeviceID = %q, want %q", got, want)
		}

		if got, want := access.SessionID, domain.SessionID("sess-1"); got != want {
			t.Errorf("SessionID = %q, want %q", got, want)
		}

		return "custom-token", nil
	})

	if _, err := service.IssueFirestoreToken(context.Background(), validConnectionInput()); err != nil {
		t.Fatalf("IssueFirestoreToken() error = %v", err)
	}
}

func TestIssueFirestoreTokenForManagementConsole(t *testing.T) {
	t.Parallel()

	service, _, minter := newTokenService(t)
	input := validTokenInput()
	input.Role = domain.PeerRoleConsole
	input.EdgeDeviceID = ""

	minter.EXPECT().Mint(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, access domain.FirestoreAccess) (string, error) {
		if access.EdgeDeviceID != "" || access.SessionID != "" {
			t.Errorf("access = %+v, want no edge or session binding", access)
		}

		return "custom-token", nil
	})

	if _, err := service.IssueFirestoreToken(context.Background(), input); err != nil {
		t.Fatalf("IssueFirestoreToken() error = %v", err)
	}
}

func TestIssueFirestoreTokenRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(input *application.IssueFirestoreTokenInput)
		wantErr error
	}{
		{name: "subject", mutate: func(i *application.IssueFirestoreTokenInput) { i.Subject = "" }, wantErr: domain.ErrSubjectRequired},
		{name: "tenant", mutate: func(i *application.IssueFirestoreTokenInput) { i.TenantID = "" }, wantErr: domain.ErrTenantIDRequired},
		{name: "event", mutate: func(i *application.IssueFirestoreTokenInput) { i.EventID = "" }, wantErr: domain.ErrEventIDRequired},
		{name: "role", mutate: func(i *application.IssueFirestoreTokenInput) { i.Role = "admin" }, wantErr: domain.ErrPeerRoleInvalid},
		{name: "expiry", mutate: func(i *application.IssueFirestoreTokenInput) { i.ExpiresAt = time.Time{} }, wantErr: domain.ErrExpiresAtRequired},
		{name: "missing edge", mutate: func(i *application.IssueFirestoreTokenInput) { i.EdgeDeviceID = "" }, wantErr: domain.ErrEdgeDeviceIDRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service, _, _ := newTokenService(t)
			input := validTokenInput()
			tt.mutate(&input)

			_, err := service.IssueFirestoreToken(context.Background(), input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("IssueFirestoreToken() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestIssueFirestoreTokenRejectsConnectionWithoutBinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(input *application.IssueFirestoreTokenInput)
		wantErr error
	}{
		{name: "edge", mutate: func(i *application.IssueFirestoreTokenInput) { i.EdgeDeviceID = "" }, wantErr: domain.ErrEdgeDeviceIDRequired},
		{name: "session", mutate: func(i *application.IssueFirestoreTokenInput) { i.SessionID = "" }, wantErr: domain.ErrSessionIDRequired},
		{name: "edge format", mutate: func(i *application.IssueFirestoreTokenInput) { i.EdgeDeviceID = "edge-1" }, wantErr: domain.ErrEdgeDeviceIDInvalid},
		{name: "session format", mutate: func(i *application.IssueFirestoreTokenInput) { i.SessionID = "not/a/session" }, wantErr: domain.ErrSessionIDInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service, _, _ := newTokenService(t)
			input := validConnectionInput()
			tt.mutate(&input)

			_, err := service.IssueFirestoreToken(context.Background(), input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("IssueFirestoreToken() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestIssueFirestoreTokenRejectsUnknownSession(t *testing.T) {
	t.Parallel()

	service, repo, _ := newTokenService(t)
	repo.EXPECT().SessionExists(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)

	_, err := service.IssueFirestoreToken(context.Background(), validConnectionInput())
	if !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("IssueFirestoreToken() error = %v, want %v", err, domain.ErrSessionNotFound)
	}
}

func TestIssueFirestoreTokenPropagatesSessionLookupError(t *testing.T) {
	t.Parallel()

	service, repo, _ := newTokenService(t)
	wantErr := errors.New("firestore unavailable")
	repo.EXPECT().SessionExists(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, wantErr)

	_, err := service.IssueFirestoreToken(context.Background(), validConnectionInput())
	if !errors.Is(err, wantErr) {
		t.Errorf("IssueFirestoreToken() error = %v, want %v", err, wantErr)
	}
}

func TestIssueFirestoreTokenPropagatesMinterError(t *testing.T) {
	t.Parallel()

	service, _, minter := newTokenService(t)
	wantErr := errors.New("signer unavailable")
	minter.EXPECT().Mint(gomock.Any(), gomock.Any()).Return("", wantErr)

	_, err := service.IssueFirestoreToken(context.Background(), validTokenInput())
	if !errors.Is(err, wantErr) {
		t.Errorf("IssueFirestoreToken() error = %v, want %v", err, wantErr)
	}
}

func TestRequestConnection(t *testing.T) {
	t.Parallel()

	service, repo := newService(t)
	repo.EXPECT().CreateSession(gomock.Any(), domain.TenantID("tenant-1"), domain.EventID("event-1"), domain.EdgeID("edge-1")).Return(domain.SessionID("sess-1"), nil)

	sessionID, err := service.RequestConnection(context.Background(), application.RequestConnectionInput{TenantID: "tenant-1", EventID: "event-1", EdgeID: "edge-1"})
	if err != nil {
		t.Fatalf("RequestConnection() error = %v", err)
	}

	if want := domain.SessionID("sess-1"); sessionID != want {
		t.Errorf("RequestConnection() = %q, want %q", sessionID, want)
	}
}

func TestRequestConnectionRejectsMissingIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   application.RequestConnectionInput
		wantErr error
	}{
		{
			name:    "tenant",
			input:   application.RequestConnectionInput{TenantID: "", EventID: "event-1", EdgeID: "edge-1"},
			wantErr: domain.ErrTenantIDRequired,
		},
		{
			name:    "event",
			input:   application.RequestConnectionInput{TenantID: "tenant-1", EventID: "", EdgeID: "edge-1"},
			wantErr: domain.ErrEventIDRequired,
		},
		{
			name:    "edge",
			input:   application.RequestConnectionInput{TenantID: "tenant-1", EventID: "event-1", EdgeID: ""},
			wantErr: domain.ErrEdgeIDRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service, _ := newService(t)

			_, err := service.RequestConnection(context.Background(), tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("RequestConnection() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequestConnectionPropagatesRepositoryError(t *testing.T) {
	t.Parallel()

	service, repo := newService(t)
	wantErr := errors.New("firestore unavailable")
	repo.EXPECT().CreateSession(gomock.Any(), domain.TenantID("tenant-1"), domain.EventID("event-1"), domain.EdgeID("edge-1")).Return(domain.SessionID(""), wantErr)

	_, err := service.RequestConnection(context.Background(), application.RequestConnectionInput{TenantID: "tenant-1", EventID: "event-1", EdgeID: "edge-1"})
	if !errors.Is(err, wantErr) {
		t.Errorf("RequestConnection() error = %v, want %v", err, wantErr)
	}
}
