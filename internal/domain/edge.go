package domain

import "errors"

var (
	ErrEdgeIDRequired = errors.New("edge id is required")
)

type EdgeID string

func NewEdgeID(value string) (EdgeID, error) {
	if value == "" {
		return "", ErrEdgeIDRequired
	}

	return EdgeID(value), nil
}

type SessionID string
