package connect

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connectrpc "connectrpc.com/connect"
	"go.uber.org/mock/gomock"

	edgebridgev1 "github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1"
	"github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1/edgebridgev1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application/applicationmock"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

func newTestRepository(t *testing.T) *applicationmock.MockEdgeRepository {
	t.Helper()

	return applicationmock.NewMockEdgeRepository(gomock.NewController(t))
}

func newSignalingService(repo application.EdgeRepository) *application.SignalingService {
	return application.NewSignalingService(repo)
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

func newRequest(tenantID, eventID, edgeID string) *connectrpc.Request[edgebridgev1.RequestConnectionRequest] {
	req := connectrpc.NewRequest(&edgebridgev1.RequestConnectionRequest{EdgeDeviceId: edgeID})
	req.Header().Set(tenantIDHeader, tenantID)
	req.Header().Set(eventIDHeader, eventID)

	return req
}

func TestRoutesServeRequestConnection(t *testing.T) {
	t.Parallel()

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), domain.TenantID("tenant-1"), domain.EventID("event-1"), domain.EdgeID("edge-1")).Return(domain.SessionID("sess-1"), nil)

	server := newTestServer(t, repo)
	client := edgebridgev1connect.NewSignalingServiceClient(server.Client(), server.URL)

	res, err := client.RequestConnection(context.Background(), newRequest("tenant-1", "event-1", "edge-1"))
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

	_, err := service.RequestConnection(context.Background(), newRequest("tenant-1", "event-1", ""))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInvalidArgument; got != want {
		t.Fatalf("RequestConnection() error code = %v, want %v", got, want)
	}
}

func TestRoutesRejectMissingTenantOrEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tenantID string
		eventID  string
	}{
		{name: "tenant", tenantID: "", eventID: "event-1"},
		{name: "event", tenantID: "tenant-1", eventID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := newTestServer(t, newTestRepository(t))
			client := edgebridgev1connect.NewSignalingServiceClient(server.Client(), server.URL)

			_, err := client.RequestConnection(context.Background(), newRequest(tt.tenantID, tt.eventID, "edge-1"))
			if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
				t.Errorf("RequestConnection() error code = %v, want %v", got, want)
			}
		})
	}
}

func TestServiceRepositoryFailureIsInternal(t *testing.T) {
	captureLog(t)

	repo := newTestRepository(t)
	repo.EXPECT().CreateSession(gomock.Any(), domain.TenantID("tenant-1"), domain.EventID("event-1"), domain.EdgeID("edge-1")).Return(domain.SessionID(""), errors.New("firestore unavailable"))
	service := NewService(newSignalingService(repo))

	_, err := service.RequestConnection(context.Background(), newRequest("tenant-1", "event-1", "edge-1"))
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeInternal; got != want {
		t.Errorf("RequestConnection() error code = %v, want %v", got, want)
	}
}
