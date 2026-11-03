package domain

import (
	"errors"
	"regexp"
)

var (
	ErrTenantIDRequired     = errors.New("tenant id is required")
	ErrEventIDRequired      = errors.New("event id is required")
	ErrEdgeIDRequired       = errors.New("edge id is required")
	ErrEdgeDeviceIDRequired = errors.New("edge device id is required")
	ErrEdgeDeviceIDInvalid  = errors.New("edge device id is invalid")
	ErrSessionIDRequired    = errors.New("session id is required")
	ErrSessionIDInvalid     = errors.New("session id is invalid")
	ErrSessionNotFound      = errors.New("session does not belong to the edge device")
)

var (
	publicIDPattern  = regexp.MustCompile(`^[0-9a-f]{16}$`)
	sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
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

func NewEdgeDeviceID(value string) (EdgeID, error) {
	if value == "" {
		return "", ErrEdgeDeviceIDRequired
	}

	if !publicIDPattern.MatchString(value) {
		return "", ErrEdgeDeviceIDInvalid
	}

	return EdgeID(value), nil
}

type SessionID string

func NewSessionID(value string) (SessionID, error) {
	if value == "" {
		return "", ErrSessionIDRequired
	}

	if !sessionIDPattern.MatchString(value) {
		return "", ErrSessionIDInvalid
	}

	return SessionID(value), nil
}
