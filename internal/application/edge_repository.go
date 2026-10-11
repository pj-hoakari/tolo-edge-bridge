package application

import (
	"context"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

//go:generate go tool mockgen -source=edge_repository.go -destination=applicationmock/edge_repository.go -package=applicationmock

type EdgeRepository interface {
	CreateSession(ctx context.Context, edgeID domain.EdgeID) (domain.SessionID, error)
}
