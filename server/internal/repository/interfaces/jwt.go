package interfaces

import (
	"context"
	"time"
)

type JWTRepository interface {
	SetJWT(ctx context.Context, uuid string, token string, expiry time.Duration) error
	GetJWT(ctx context.Context, uuid string) (string, error)
	DelJWT(ctx context.Context, uuid string) error
	ToBlackList(ctx context.Context, token string, ttl time.Duration) error
	IsBlackListed(ctx context.Context, token string) bool
}
