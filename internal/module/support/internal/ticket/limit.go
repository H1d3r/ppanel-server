package ticket

import (
	"context"
	"strconv"

	"github.com/perfect-panel/server/internal/auth/ratelimit"
	"github.com/redis/go-redis/v9"
)

// Every new ticket also opens a forum topic in the Telegram admin group, so
// one user may open only a handful per hour.
const (
	creationWindowSeconds = 3600
	creationQuota         = 5
	creationKeyPrefix     = "ticket:limit:user:"
)

// CreationLimiter caps how many tickets one user may open per window.
type CreationLimiter interface {
	// Allow takes one permit for the user and reports whether it was
	// granted.
	Allow(ctx context.Context, userID int64) (bool, error)
}

// NewCreationLimiter backs the limit with Redis; a nil client disables it.
func NewCreationLimiter(rds *redis.Client) CreationLimiter {
	if rds == nil {
		return nil
	}
	return periodCreationLimiter{limit: ratelimit.NewPeriodLimit(creationWindowSeconds, creationQuota, rds, creationKeyPrefix)}
}

type periodCreationLimiter struct {
	limit *ratelimit.PeriodLimit
}

func (l periodCreationLimiter) Allow(ctx context.Context, userID int64) (bool, error) {
	state, err := l.limit.TakeCtx(ctx, strconv.FormatInt(userID, 10))
	if err != nil {
		return false, err
	}
	return l.limit.ParsePermitState(state), nil
}
