package domain

import (
	"errors"
	"time"
)

const AliveWindow = 30 * time.Second

var (
	ErrEdgeIDRequired = errors.New("edge id is required")
	ErrEdgeNotFound   = errors.New("edge not found")
)

type EdgeID string

func NewEdgeID(value string) (EdgeID, error) {
	if value == "" {
		return "", ErrEdgeIDRequired
	}

	return EdgeID(value), nil
}

type Edge struct {
	ID         EdgeID
	LastSeenAt time.Time
}

type SessionID string

func AliveCutoff(now time.Time) time.Time {
	return now.Add(-AliveWindow)
}
