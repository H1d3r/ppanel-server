// Package usersession issues, validates and ends user sessions. A session is
// a signed token plus a Redis record under its session id. Every token also
// carries the user's session epoch from when it was issued; revoking replaces
// the epoch, which invalidates every token issued before. Password changes
// and resets revoke, so a stolen session does not outlive them.
package usersession

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/perfect-panel/server/internal/auth/devicesession"
	"github.com/perfect-panel/server/internal/auth/token"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/redis/go-redis/v9"
)

// The claims of a session token.
const (
	UserIDClaim    = "UserId"
	SessionIDClaim = "SessionId"
	LoginTypeClaim = "LoginType"
	// EpochClaim carries the user's session epoch.
	EpochClaim = "SessionEpoch"
)

// An epoch records whether it came from a revocation, so a token issued
// before epochs existed stays valid only until the first revocation.
const (
	issuedPrefix  = "i:"
	revokedPrefix = "r:"
)

var (
	// ErrUnavailable reports a missing session store.
	ErrUnavailable = errors.New("user session store unavailable")
	// ErrInvalidToken reports a token whose signature or lifetime does not
	// verify.
	ErrInvalidToken = errors.New("session token is invalid or expired")
	// ErrNotSession reports a verified token that is not a session: other
	// tokens, order event tickets for one, are signed with the same secret.
	ErrNotSession = errors.New("token is not a session")
	// ErrEnded reports a session that was ended or has expired.
	ErrEnded = errors.New("session ended")
	// ErrRevoked reports a session issued before the user's latest
	// revocation.
	ErrRevoked = errors.New("session revoked")
	// ErrDeviceSession reports device claims that are malformed or missing
	// from a device session; the client has to sign in again.
	ErrDeviceSession = errors.New("device session must be renewed")
	// ErrDeviceRevoked reports a device session whose device was revoked.
	ErrDeviceRevoked = errors.New("device session revoked")
)

// Store is the Redis surface issuing and revoking sessions uses;
// *redis.Client satisfies it.
type Store interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
}

// ValidationStore adds the batched read validating a session uses.
type ValidationStore interface {
	Store
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
}

// Deleter is the Redis surface ending a session uses.
type Deleter interface {
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// SessionKey is the Redis key recording the session sessionID; it holds the
// user id until the session ends.
func SessionKey(sessionID string) string {
	return config.SessionIdKey + ":" + sessionID
}

// Key is the Redis key holding the user's session epoch; validation reads it
// together with the session itself.
func Key(userID int64) string { return "auth:user_session_epoch:" + strconv.FormatInt(userID, 10) }

// missing reports a store that is absent, including a nil *redis.Client
// passed as an interface.
func missing(client any) bool {
	if client == nil {
		return true
	}
	rdb, ok := client.(*redis.Client)
	return ok && rdb == nil
}

// Grant describes a session to issue.
type Grant struct {
	UserID int64
	// LoginType records how the user signed in; "device" marks a session of
	// the device transport.
	LoginType string
	// DeviceID binds the session to a device, so revoking the device ends it;
	// 0 issues a session bound to no device.
	DeviceID int64
}

// Issue signs a session token for grant that lives for lifetime seconds and
// records the session.
func Issue(ctx context.Context, client Store, secret string, lifetime int64, grant Grant) (string, error) {
	if missing(client) || lifetime <= 0 {
		return "", ErrUnavailable
	}
	if grant.UserID <= 0 {
		return "", errors.New("a session needs a user")
	}
	sessionID := uuid.NewV7().String()
	epoch, err := AcquireEpoch(ctx, client, grant.UserID)
	if err != nil {
		return "", fmt.Errorf("acquire session epoch: %w", err)
	}
	options := []token.Option{
		token.WithOption(UserIDClaim, grant.UserID),
		token.WithOption(SessionIDClaim, sessionID),
		token.WithOption(LoginTypeClaim, grant.LoginType),
		token.WithOption(EpochClaim, epoch),
	}
	if grant.DeviceID > 0 {
		deviceEpoch, err := devicesession.AcquireEpoch(ctx, client, grant.DeviceID)
		if err != nil {
			return "", fmt.Errorf("acquire device session epoch: %w", err)
		}
		options = append(options,
			token.WithOption(devicesession.IDClaim, strconv.FormatInt(grant.DeviceID, 10)),
			token.WithOption(devicesession.EpochClaim, deviceEpoch))
	}
	signed, err := token.NewJwtToken(secret, timeutil.Now().Unix(), lifetime, options...)
	if err != nil {
		return "", fmt.Errorf("sign session token: %w", err)
	}
	if err := client.Set(ctx, SessionKey(sessionID), grant.UserID, time.Duration(lifetime)*time.Second).Err(); err != nil {
		return "", fmt.Errorf("record session: %w", err)
	}
	return signed, nil
}

// Claims are the claims of a live session.
type Claims struct {
	UserID    int64
	SessionID string
	LoginType string
	// DeviceID is the device the session is bound to; 0 for a session bound
	// to no device. The caller still checks that the device is enabled and
	// belongs to the user.
	DeviceID int64
}

// Validate checks a session token: its signature and lifetime, that it is a
// session at all, that the session was not ended, the user's session epoch
// and, for a device session, the device's epoch.
func Validate(ctx context.Context, client ValidationStore, secret, signed string) (*Claims, error) {
	if missing(client) {
		return nil, ErrUnavailable
	}
	claims, err := token.ParseJwtToken(signed, secret)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	// Every claim is checked before use: a verified token is not
	// necessarily a session.
	loginType, _ := claims[LoginTypeClaim].(string)
	rawUserID, hasUser := claims[UserIDClaim].(float64)
	sessionID, hasSession := claims[SessionIDClaim].(string)
	if !hasUser || !hasSession || sessionID == "" {
		return nil, ErrNotSession
	}
	userID := int64(rawUserID)
	values, err := client.MGet(ctx, SessionKey(sessionID), Key(userID)).Result()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if value, _ := values[0].(string); value != strconv.FormatInt(userID, 10) {
		return nil, ErrEnded
	}
	epoch, _ := values[1].(string)
	if err := Check(claims, epoch); err != nil {
		return nil, err
	}
	deviceID, deviceEpoch, err := devicesession.Binding(claims)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeviceSession, err)
	}
	if deviceID != 0 {
		current, err := devicesession.Epoch(ctx, client, deviceID)
		if err != nil || deviceEpoch != current {
			return nil, ErrDeviceRevoked
		}
	}
	return &Claims{UserID: userID, SessionID: sessionID, LoginType: loginType, DeviceID: deviceID}, nil
}

// End ends one session; the user's other sessions stay valid.
func End(ctx context.Context, client Deleter, sessionID string) error {
	if missing(client) {
		return ErrUnavailable
	}
	return client.Del(ctx, SessionKey(sessionID)).Err()
}

// AcquireEpoch returns the epoch a new session of the user carries, creating
// it on first use. The key never expires: an evicted epoch fails every token
// that carries one closed.
func AcquireEpoch(ctx context.Context, client Store, userID int64) (string, error) {
	if missing(client) || userID <= 0 {
		return "", ErrUnavailable
	}
	if err := client.SetNX(ctx, Key(userID), issuedPrefix+uuid.NewV7().String(), 0).Err(); err != nil {
		return "", err
	}
	return client.Get(ctx, Key(userID)).Result()
}

// Revoke invalidates every session issued to the user so far.
func Revoke(ctx context.Context, client Store, userID int64) error {
	if missing(client) || userID <= 0 {
		return ErrUnavailable
	}
	return client.Set(ctx, Key(userID), revokedPrefix+uuid.NewV7().String(), 0).Err()
}

// Check reports whether a token's claims are still valid against the user's
// current epoch (empty when none is stored). A token issued before epochs
// existed carries none and stays valid until the user is first revoked.
func Check(claims map[string]any, current string) error {
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
