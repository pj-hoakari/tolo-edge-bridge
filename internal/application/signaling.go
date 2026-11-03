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
	Subject      string
	TenantID     string
	EventID      string
	Role         domain.PeerRole
	EdgeDeviceID string
	SessionID    string
	Connection   bool
	ExpiresAt    time.Time
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
	if input.Subject == "" {
		return FirestoreToken{}, domain.ErrSubjectRequired
	}

	tenantID, err := domain.NewTenantID(input.TenantID)
	if err != nil {
		return FirestoreToken{}, err
	}

	eventID, err := domain.NewEventID(input.EventID)
	if err != nil {
		return FirestoreToken{}, err
	}

	if input.Role != domain.PeerRoleEdge && input.Role != domain.PeerRoleConsole {
		return FirestoreToken{}, fmt.Errorf("%w: %q", domain.ErrPeerRoleInvalid, input.Role)
	}

	if input.ExpiresAt.IsZero() {
		return FirestoreToken{}, domain.ErrExpiresAtRequired
	}

	edgeDeviceID, sessionID, err := s.firestoreBinding(ctx, tenantID, eventID, input)
	if err != nil {
		return FirestoreToken{}, err
	}

	access, err := domain.NewFirestoreAccess(input.Subject, tenantID, eventID, input.Role, edgeDeviceID, sessionID, input.ExpiresAt)
	if err != nil {
		return FirestoreToken{}, err
	}

	token, err := s.tokens.Mint(ctx, access)
	if err != nil {
		return FirestoreToken{}, fmt.Errorf("mint firestore token for %s role: %w", access.Role, err)
	}

	return FirestoreToken{Token: token, ExpiresAt: access.ExpiresAt}, nil
}

func (s *SignalingService) firestoreBinding(ctx context.Context, tenantID domain.TenantID, eventID domain.EventID, input IssueFirestoreTokenInput) (domain.EdgeID, domain.SessionID, error) {
	if input.Connection {
		edgeDeviceID, err := domain.NewEdgeDeviceID(input.EdgeDeviceID)
		if err != nil {
			return "", "", err
		}

		sessionID, err := domain.NewSessionID(input.SessionID)
		if err != nil {
			return "", "", err
		}

		exists, err := s.edges.SessionExists(ctx, tenantID, eventID, edgeDeviceID, sessionID)
		if err != nil {
			return "", "", fmt.Errorf("verify session for edge %q: %w", edgeDeviceID, err)
		}

		if !exists {
			return "", "", domain.ErrSessionNotFound
		}

		return edgeDeviceID, sessionID, nil
	}

	if input.Role == domain.PeerRoleEdge {
		if input.EdgeDeviceID == "" {
			return "", "", domain.ErrEdgeDeviceIDRequired
		}

		return domain.EdgeID(input.EdgeDeviceID), "", nil
	}

	return "", "", nil
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
