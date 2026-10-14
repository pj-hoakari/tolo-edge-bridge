package firestore

import (
	"context"
	"fmt"
	"time"

	gcpfirestore "cloud.google.com/go/firestore"

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
	ref := r.client.Collection(tenantsCollection).Doc(string(tenantID)).
		Collection(eventsCollection).Doc(string(eventID)).
		Collection(edgeDevicesCollection).Doc(string(edgeID)).
		Collection(sessionsCollection).NewDoc()

	if _, err := ref.Set(ctx, sessionDoc{Status: sessionStatusRequested, CreatedAt: time.Time{}}); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return domain.SessionID(ref.ID), nil
}
