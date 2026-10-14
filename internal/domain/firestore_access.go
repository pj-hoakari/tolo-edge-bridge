package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrSubjectRequired   = errors.New("subject is required")
	ErrPeerRoleInvalid   = errors.New("peer role is invalid")
	ErrExpiresAtRequired = errors.New("expires at is required")
)

type PeerRole string

const (
	PeerRoleEdge    PeerRole = "edge"
	PeerRoleConsole PeerRole = "console"
)

type FirestoreAccess struct {
	Subject   string
	TenantID  TenantID
	EventID   EventID
	Role      PeerRole
	ExpiresAt time.Time
}

func NewFirestoreAccess(subject string, tenantID TenantID, eventID EventID, role PeerRole, expiresAt time.Time) (FirestoreAccess, error) {
	if subject == "" {
		return FirestoreAccess{}, ErrSubjectRequired
	}

	if tenantID == "" {
		return FirestoreAccess{}, ErrTenantIDRequired
	}

	if eventID == "" {
		return FirestoreAccess{}, ErrEventIDRequired
	}

	if role != PeerRoleEdge && role != PeerRoleConsole {
		return FirestoreAccess{}, fmt.Errorf("%w: %q", ErrPeerRoleInvalid, role)
	}

	if expiresAt.IsZero() {
		return FirestoreAccess{}, ErrExpiresAtRequired
	}

	return FirestoreAccess{
		Subject:   subject,
		TenantID:  tenantID,
		EventID:   eventID,
		Role:      role,
		ExpiresAt: expiresAt.Truncate(time.Second).UTC(),
	}, nil
}
