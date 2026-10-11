package firestore

import (
	"context"
	"fmt"
	"time"

	gcpfirestore "cloud.google.com/go/firestore"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

const (
	edgesCollection        = "edges"
	sessionsSubcollection  = "sessions"
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

func (r *EdgeRepository) CreateSession(ctx context.Context, edgeID domain.EdgeID) (domain.SessionID, error) {
	ref := r.client.Collection(edgesCollection).Doc(string(edgeID)).Collection(sessionsSubcollection).NewDoc()

	if _, err := ref.Set(ctx, sessionDoc{Status: sessionStatusRequested, CreatedAt: time.Time{}}); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return domain.SessionID(ref.ID), nil
}
