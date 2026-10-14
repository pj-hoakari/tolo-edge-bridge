package domain

import "errors"

var (
	ErrTenantIDRequired = errors.New("tenant id is required")
	ErrEventIDRequired  = errors.New("event id is required")
	ErrEdgeIDRequired   = errors.New("edge id is required")
)

type TenantID string

func NewTenantID(value string) (TenantID, error) {
	if value == "" {
		return "", ErrTenantIDRequired
	}

	return TenantID(value), nil
}

type EventID string

func NewEventID(value string) (EventID, error) {
	if value == "" {
		return "", ErrEventIDRequired
	}

	return EventID(value), nil
}

type EdgeID string

func NewEdgeID(value string) (EdgeID, error) {
	if value == "" {
		return "", ErrEdgeIDRequired
	}

	return EdgeID(value), nil
}

type SessionID string
