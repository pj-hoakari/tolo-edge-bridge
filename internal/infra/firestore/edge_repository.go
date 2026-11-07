package firestore

import (
	"context"
	"fmt"
	"time"

	gcpfirestore "cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

const (
	tenantsCollection      = "tenants"
	eventsCollection       = "events"
	edgeDevicesCollection  = "edgeDevices"
	sessionsCollection     = "sessions"
	sessionStatusRequested = "requested"
)

type sessionDoc struct {
	Status    string    `firestore:"status"`
	CreatedAt time.Time `firestore:"createdAt,serverTimestamp"`
}

type EdgeRepository struct {
	client *gcpfirestore.Client
}

func NewEdgeRepository(client *gcpfirestore.Client) *EdgeRepository {
	return &EdgeRepository{client: client}
}

func (r *EdgeRepository) CreateSession(ctx context.Context, tenantID domain.TenantID, eventID domain.EventID, edgeID domain.EdgeID) (domain.SessionID, error) {
	ref := r.sessionRef(tenantID, eventID, edgeID).NewDoc()

	if _, err := ref.Set(ctx, sessionDoc{Status: sessionStatusRequested, CreatedAt: time.Time{}}); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return domain.SessionID(ref.ID), nil
}

func (r *EdgeRepository) SessionExists(ctx context.Context, tenantID domain.TenantID, eventID domain.EventID, edgeID domain.EdgeID, sessionID domain.SessionID) (bool, error) {
	snapshot, err := r.sessionRef(tenantID, eventID, edgeID).Doc(string(sessionID)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}

		return false, fmt.Errorf("get session %q: %w", sessionID, err)
	}

	return snapshot.Exists(), nil
}

func (r *EdgeRepository) sessionRef(tenantID domain.TenantID, eventID domain.EventID, edgeID domain.EdgeID) *gcpfirestore.CollectionRef {
	return r.client.Collection(tenantsCollection).Doc(string(tenantID)).
		Collection(eventsCollection).Doc(string(eventID)).
		Collection(edgeDevicesCollection).Doc(string(edgeID)).
		Collection(sessionsCollection)
}
