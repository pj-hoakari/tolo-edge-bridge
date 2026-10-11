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

var now = time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

func newService(t *testing.T) (*application.SignalingService, *applicationmock.MockEdgeRepository) {
	t.Helper()

	repo := applicationmock.NewMockEdgeRepository(gomock.NewController(t))

	return application.NewSignalingService(repo, func() time.Time { return now }), repo
}

func TestListAliveEdges(t *testing.T) {
	t.Parallel()

	service, repo := newService(t)
	alive := []domain.Edge{{ID: "edge-1", LastSeenAt: now.Add(-10 * time.Second)}}
	repo.EXPECT().ListEdgesSeenAfter(gomock.Any(), now.Add(-30*time.Second)).Return(alive, nil)

	edges, err := service.ListAliveEdges(context.Background())
	if err != nil {
		t.Fatalf("ListAliveEdges() error = %v", err)
	}

	if len(edges) != 1 || edges[0] != alive[0] {
		t.Errorf("ListAliveEdges() = %v, want %v", edges, alive)
	}
}

func TestListAliveEdgesPropagatesRepositoryError(t *testing.T) {
	t.Parallel()

	service, repo := newService(t)
	wantErr := errors.New("firestore unavailable")
	repo.EXPECT().ListEdgesSeenAfter(gomock.Any(), gomock.Any()).Return(nil, wantErr)

	if _, err := service.ListAliveEdges(context.Background()); !errors.Is(err, wantErr) {
		t.Errorf("ListAliveEdges() error = %v, want %v", err, wantErr)
	}
}

func TestRequestConnection(t *testing.T) {
	t.Parallel()

	service, repo := newService(t)
	repo.EXPECT().CreateSession(gomock.Any(), domain.EdgeID("edge-1")).Return(domain.SessionID("sess-1"), nil)

	sessionID, err := service.RequestConnection(context.Background(), application.RequestConnectionInput{EdgeID: "edge-1"})
	if err != nil {
		t.Fatalf("RequestConnection() error = %v", err)
	}

	if want := domain.SessionID("sess-1"); sessionID != want {
		t.Errorf("RequestConnection() = %q, want %q", sessionID, want)
	}
}

func TestRequestConnectionRejectsEmptyEdgeID(t *testing.T) {
	t.Parallel()

	service, _ := newService(t)

	_, err := service.RequestConnection(context.Background(), application.RequestConnectionInput{EdgeID: ""})
	if !errors.Is(err, domain.ErrEdgeIDRequired) {
		t.Errorf("RequestConnection() error = %v, want %v", err, domain.ErrEdgeIDRequired)
	}
}

func TestRequestConnectionPropagatesRepositoryError(t *testing.T) {
	t.Parallel()

	service, repo := newService(t)
	wantErr := errors.New("firestore unavailable")
	repo.EXPECT().CreateSession(gomock.Any(), domain.EdgeID("edge-1")).Return(domain.SessionID(""), wantErr)

	_, err := service.RequestConnection(context.Background(), application.RequestConnectionInput{EdgeID: "edge-1"})
	if !errors.Is(err, wantErr) {
		t.Errorf("RequestConnection() error = %v, want %v", err, wantErr)
	}
}
