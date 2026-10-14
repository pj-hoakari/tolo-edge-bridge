package application

import (
	"context"
	"fmt"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

type RequestConnectionInput struct {
	TenantID string
	EventID  string
	EdgeID   string
}

type SignalingService struct {
	edges EdgeRepository
}

func NewSignalingService(edges EdgeRepository) *SignalingService {
	return &SignalingService{edges: edges}
}

func (s *SignalingService) RequestConnection(ctx context.Context, input RequestConnectionInput) (domain.SessionID, error) {
	tenantID, err := domain.NewTenantID(input.TenantID)
	if err != nil {
		return "", err
	}

	eventID, err := domain.NewEventID(input.EventID)
	if err != nil {
		return "", err
	}

	edgeID, err := domain.NewEdgeID(input.EdgeID)
	if err != nil {
		return "", err
	}

	sessionID, err := s.edges.CreateSession(ctx, tenantID, eventID, edgeID)
	if err != nil {
		return "", fmt.Errorf("create session for edge %q: %w", edgeID, err)
	}

	return sessionID, nil
}
