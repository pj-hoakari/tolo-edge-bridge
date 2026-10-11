package application

import (
	"context"
	"time"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

//go:generate go tool mockgen -source=edge_repository.go -destination=applicationmock/edge_repository.go -package=applicationmock

type EdgeRepository interface {
	ListEdgesSeenAfter(ctx context.Context, cutoff time.Time) ([]domain.Edge, error)
	EdgeExists(ctx context.Context, edgeID domain.EdgeID) (bool, error)
	CreateSession(ctx context.Context, edgeID domain.EdgeID) (domain.SessionID, error)
}
