package application

import (
	"context"

	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
)

//go:generate go tool mockgen -source=firestore_token_minter.go -destination=applicationmock/firestore_token_minter.go -package=applicationmock

type FirestoreTokenMinter interface {
	Mint(ctx context.Context, access domain.FirestoreAccess) (string, error)
}
