package application_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application/applicationmock"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

func newService(t *testing.T) (*application.SignalingService, *applicationmock.MockEdgeRepository) {
	t.Helper()

	repo := applicationmock.NewMockEdgeRepository(gomock.NewController(t))

	return application.NewSignalingService(repo), repo
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
