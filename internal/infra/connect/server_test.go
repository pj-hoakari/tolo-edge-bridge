package connect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	"go.uber.org/mock/gomock"

	signalingv1 "github.com/pj-hoakari/tolo-edge-bridge/gen/signaling/v1"
	"github.com/pj-hoakari/tolo-edge-bridge/gen/signaling/v1/signalingv1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application/applicationmock"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

func newTestRepository(t *testing.T) *applicationmock.MockEdgeRepository {
	t.Helper()

	return applicationmock.NewMockEdgeRepository(gomock.NewController(t))
}

func newSignalingService(repo application.EdgeRepository) *application.SignalingService {
	return application.NewSignalingService(repo, time.Now)
}

func newTestServer(t *testing.T, repo application.EdgeRepository) *httptest.Server {
	t.Helper()

	routes, err := Routes(newSignalingService(repo))
	if err != nil {
		t.Fatalf("Routes() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func TestRoutesServeListAliveEdges(t *testing.T) {
	t.Parallel()

	repo := newTestRepository(t)
	seen := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	repo.EXPECT().ListEdgesSeenAfter(gomock.Any(), gomock.Any()).
		Return([]domain.Edge{{ID: "edge-1", LastSeenAt: seen}}, nil)

	server := newTestServer(t, repo)
	client := signalingv1connect.NewEdgeRegistryServiceClient(server.Client(), server.URL)

	res, err := client.ListAliveEdges(context.Background(), connectrpc.NewRequest(&signalingv1.ListAliveEdgesRequest{}))
	if err != nil {
		t.Fatalf("ListAliveEdges() error = %v", err)
	}

	edges := res.Msg.GetEdges()
	if len(edges) != 1 {
		t.Fatalf("edges = %v, want one edge", edges)
	}

	if got, want := edges[0].GetId(), "edge-1"; got != want {
		t.Errorf("Id = %q, want %q", got, want)
	}

	if got := edges[0].GetLastSeenAt().AsTime(); !got.Equal(seen) {
		t.Errorf("LastSeenAt = %v, want %v", got, seen)
	}
}

func TestRoutesServeRequestConnection(t *testing.T) {
	t.Parallel()

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), domain.EdgeID("edge-1")).Return(domain.SessionID("sess-1"), nil)

	server := newTestServer(t, repo)
	client := signalingv1connect.NewSignalingServiceClient(server.Client(), server.URL)

	res, err := client.RequestConnection(context.Background(), connectrpc.NewRequest(&signalingv1.RequestConnectionRequest{EdgeId: "edge-1"}))
	if err != nil {
		t.Fatalf("RequestConnection() error = %v", err)
	}

	if got, want := res.Msg.GetSessionId(), "sess-1"; got != want {
		t.Errorf("SessionId = %q, want %q", got, want)
	}
}

func TestServiceRequestConnectionRejectsEmptyEdgeID(t *testing.T) {
	t.Parallel()

	service := NewService(newSignalingService(newTestRepository(t)))

	_, err := service.RequestConnection(context.Background(), connectrpc.NewRequest(&signalingv1.RequestConnectionRequest{EdgeId: ""}))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInvalidArgument; got != want {
		t.Fatalf("RequestConnection() error code = %v, want %v", got, want)
	}
}

func TestServiceRepositoryFailureIsInternal(t *testing.T) {
	captureLog(t)

	repo := newTestRepository(t)
	repo.EXPECT().ListEdgesSeenAfter(gomock.Any(), gomock.Any()).Return(nil, errors.New("firestore unavailable"))
	repo.EXPECT().CreateSession(gomock.Any(), domain.EdgeID("edge-1")).Return(domain.SessionID(""), errors.New("firestore unavailable"))
	service := NewService(newSignalingService(repo))

	_, err := service.ListAliveEdges(context.Background(), connectrpc.NewRequest(&signalingv1.ListAliveEdgesRequest{}))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInternal; got != want {
		t.Errorf("ListAliveEdges() error code = %v, want %v", got, want)
	}

	_, err = service.RequestConnection(context.Background(), connectrpc.NewRequest(&signalingv1.RequestConnectionRequest{EdgeId: "edge-1"}))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInternal; got != want {
		t.Errorf("RequestConnection() error code = %v, want %v", got, want)
	}
}
