package application

import (
	"context"
	"fmt"
	"time"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

type RequestConnectionInput struct {
	EdgeID string
}

type SignalingService struct {
	edges EdgeRepository
	now   func() time.Time
}

func NewSignalingService(edges EdgeRepository, now func() time.Time) *SignalingService {
	return &SignalingService{edges: edges, now: now}
}

func (s *SignalingService) ListAliveEdges(ctx context.Context) ([]domain.Edge, error) {
	edges, err := s.edges.ListEdgesSeenAfter(ctx, domain.AliveCutoff(s.now()))
	if err != nil {
		return nil, fmt.Errorf("list alive edges: %w", err)
	}

	return edges, nil
}

func (s *SignalingService) RequestConnection(ctx context.Context, input RequestConnectionInput) (domain.SessionID, error) {
	edgeID, err := domain.NewEdgeID(input.EdgeID)
	if err != nil {
		return "", err
	}

	sessionID, err := s.edges.CreateSession(ctx, edgeID)
	if err != nil {
		return "", fmt.Errorf("create session for edge %q: %w", edgeID, err)
	}

	return sessionID, nil
}
