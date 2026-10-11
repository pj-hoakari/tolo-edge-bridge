package application

import (
	"context"
	"fmt"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

type RequestConnectionInput struct {
	EdgeID string
}

type SignalingService struct {
	edges EdgeRepository
}

func NewSignalingService(edges EdgeRepository) *SignalingService {
	return &SignalingService{edges: edges}
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
