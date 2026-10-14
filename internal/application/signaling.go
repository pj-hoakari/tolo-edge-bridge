package application

import (
	"context"
	"fmt"
	"time"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

type RequestConnectionInput struct {
	TenantID string
	EventID  string
	EdgeID   string
}

type IssueFirestoreTokenInput struct {
	Subject   string
	TenantID  string
	EventID   string
	Role      domain.PeerRole
	ExpiresAt time.Time
}

type FirestoreToken struct {
	Token     string
	ExpiresAt time.Time
}

type SignalingService struct {
	edges  EdgeRepository
	tokens FirestoreTokenMinter
}

func NewSignalingService(edges EdgeRepository, tokens FirestoreTokenMinter) *SignalingService {
	return &SignalingService{edges: edges, tokens: tokens}
}

func (s *SignalingService) IssueFirestoreToken(ctx context.Context, input IssueFirestoreTokenInput) (FirestoreToken, error) {
	access, err := domain.NewFirestoreAccess(input.Subject, domain.TenantID(input.TenantID), domain.EventID(input.EventID), input.Role, input.ExpiresAt)
	if err != nil {
		return FirestoreToken{}, err
	}

	token, err := s.tokens.Mint(ctx, access)
	if err != nil {
		return FirestoreToken{}, fmt.Errorf("mint firestore token for %s role: %w", access.Role, err)
	}

	return FirestoreToken{Token: token, ExpiresAt: access.ExpiresAt}, nil
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
