package authn

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

// Password guessing against one account is capped: after maxLoginFailures
// wrong passwords within loginFailureWindow, password sign-in to that account
// is refused until the window ends. The counter is keyed by the account, so
// spreading requests across addresses does not help, and the window starts
// at the first failure, so a lockout never outlasts it. Signing in with a
// code and resetting the password stay open to the owner.
const (
	maxLoginFailures   = 10
	loginFailureWindow = 15 * time.Minute
)

func loginFailureKey(userID int64) string {
	return "auth:login_failures:" + strconv.FormatInt(userID, 10)
}

// ensureLoginAllowed refuses password sign-in to an account that used up its
// attempts in the current window.
func ensureLoginAllowed(ctx context.Context, client *redis.Client, userID int64) error {
	if client == nil {
		return nil
	}
	failures, err := client.Get(ctx, loginFailureKey(userID)).Int64()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "read sign-in attempts")
	}
	if failures >= maxLoginFailures {
		return fmt.Errorf("too many failed sign-in attempts, try again later: %w", xerr.NewErrCode(xerr.TooManyRequests))
	}
	return nil
}

// recordLoginFailure counts a wrong password.
func recordLoginFailure(ctx context.Context, client *redis.Client, userID int64) {
	if client == nil {
		return
	}
	key := loginFailureKey(userID)
	pipe := client.TxPipeline()
	pipe.SetNX(ctx, key, 0, loginFailureWindow)
	pipe.Incr(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		logger.WithContext(ctx).Errorw("[Login] record failed sign-in attempt failed", logger.Field("error", err.Error()), logger.Field("user_id", userID))
	}
}

// clearLoginFailures forgets the failures once the owner proves access.
func clearLoginFailures(ctx context.Context, client *redis.Client, userID int64) {
	if client == nil {
		return
	}
	if err := client.Del(ctx, loginFailureKey(userID)).Err(); err != nil {
		logger.WithContext(ctx).Errorw("[Login] clear failed sign-in attempts failed", logger.Field("error", err.Error()), logger.Field("user_id", userID))
	}
}
