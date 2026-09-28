// Package usersession revokes all of a user's sessions at once. Every session
// token carries the user's session epoch from when it was issued; revoking
// replaces the epoch, which invalidates every token issued before. Password
// changes and resets revoke, so a stolen session does not outlive them.
package usersession

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/redis/go-redis/v9"
)

// EpochClaim is the token claim carrying the session epoch.
const EpochClaim = "SessionEpoch"

// An epoch records whether it came from a revocation, so a token issued
// before epochs existed stays valid only until the first revocation.
const (
	issuedPrefix  = "i:"
	revokedPrefix = "r:"
)

// ErrRevoked reports a session issued before the user's latest revocation.
var ErrRevoked = errors.New("session revoked")

// Store is the Redis surface the epochs use; *redis.Client satisfies it.
type Store interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
}

// Key is the Redis key holding the user's session epoch; the auth middleware
// reads it together with the session itself.
func Key(userID int64) string { return "auth:user_session_epoch:" + strconv.FormatInt(userID, 10) }

// AcquireEpoch returns the epoch a new session of the user carries, creating
// it on first use. The key never expires: an evicted epoch fails every token
// that carries one closed.
func AcquireEpoch(ctx context.Context, client Store, userID int64) (string, error) {
	if client == nil || userID <= 0 {
		return "", errors.New("user session store unavailable")
	}
	if err := client.SetNX(ctx, Key(userID), issuedPrefix+uuid.NewV7().String(), 0).Err(); err != nil {
		return "", err
	}
	return client.Get(ctx, Key(userID)).Result()
}

// Revoke invalidates every session issued to the user so far.
func Revoke(ctx context.Context, client Store, userID int64) error {
	if client == nil || userID <= 0 {
		return errors.New("user session store unavailable")
	}
	return client.Set(ctx, Key(userID), revokedPrefix+uuid.NewV7().String(), 0).Err()
}

// Check reports whether a token's claims are still valid against the user's
// current epoch (empty when none is stored). A token issued before epochs
// existed carries none and stays valid until the user is first revoked.
func Check(claims map[string]interface{}, current string) error {
	value, ok := claims[EpochClaim]
	if !ok {
		if strings.HasPrefix(current, revokedPrefix) {
			return ErrRevoked
		}
		return nil
	}
	epoch, ok := value.(string)
	if !ok || epoch == "" || epoch != current {
		return ErrRevoked
	}
	return nil
}
