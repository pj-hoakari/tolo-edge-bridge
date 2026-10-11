package firestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	gcpfirestore "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

const (
	edgesCollection        = "edges"
	sessionsSubcollection  = "sessions"
	lastSeenAtField        = "lastSeenAt"
	sessionStatusRequested = "requested"
)

type edgeDoc struct {
	LastSeenAt time.Time `firestore:"lastSeenAt"`
}

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

func (r *EdgeRepository) ListEdgesSeenAfter(ctx context.Context, cutoff time.Time) ([]domain.Edge, error) {
	iter := r.client.Collection(edgesCollection).Where(lastSeenAtField, ">", cutoff).Documents(ctx)
	defer iter.Stop()

	edges := make([]domain.Edge, 0)

	for {
		doc, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return edges, nil
		}

		if err != nil {
			return nil, fmt.Errorf("query edges: %w", err)
		}

		var data edgeDoc
		if err := doc.DataTo(&data); err != nil {
			continue
		}

		edges = append(edges, domain.Edge{ID: domain.EdgeID(doc.Ref.ID), LastSeenAt: data.LastSeenAt})
	}
}

func (r *EdgeRepository) EdgeExists(ctx context.Context, edgeID domain.EdgeID) (bool, error) {
	_, err := r.client.Collection(edgesCollection).Doc(string(edgeID)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("get edge: %w", err)
	}

	return true, nil
}

func (r *EdgeRepository) CreateSession(ctx context.Context, edgeID domain.EdgeID) (domain.SessionID, error) {
	ref := r.client.Collection(edgesCollection).Doc(string(edgeID)).Collection(sessionsSubcollection).NewDoc()

	if _, err := ref.Set(ctx, sessionDoc{Status: sessionStatusRequested, CreatedAt: time.Time{}}); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return domain.SessionID(ref.ID), nil
}
