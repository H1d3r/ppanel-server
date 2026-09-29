package account

import (
	"context"
	"strconv"
	"time"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

// Password guessing against one account is capped: after MaxPasswordAttempts
// checks without a correct password within PasswordAttemptWindow, every flow
// that checks the account's password (sign-in, changing it, replacing a
// bound address with it) is refused until the window ends. The counter is
// keyed by the account, so spreading requests across addresses does not
// help, and the window starts at the first attempt, so a lockout never
// outlasts it. Signing in with a code and resetting the password stay open
// to the owner.
const (
	MaxPasswordAttempts   = 10
	PasswordAttemptWindow = 15 * time.Minute
)

// reserveAttemptScript counts the attempt and reports whether it is within
// the limit. Counting before the check, in one Redis script, is what makes
// the limit hold against concurrent guesses: a burst cannot read a counter
// of zero many times while its password checks queue up.
var reserveAttemptScript = redis.NewScript(`
local attempts = redis.call("INCR", KEYS[1])
if attempts == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
if attempts > tonumber(ARGV[1]) then
  return 0
end
return 1
`)

// PasswordAttemptKey is the Redis key counting the password attempts against
// the account userID in the current window.
func PasswordAttemptKey(userID int64) string {
	return "auth:password_attempts:" + strconv.FormatInt(userID, 10)
}

// ReservePasswordAttempt reserves one password check against the account
// userID and refuses once the window's attempts are used up. A flow calls it
// before it compares the password; only ClearPasswordAttempts, after a
// correct password, gives the attempts back before the window ends.
func ReservePasswordAttempt(ctx context.Context, client *redis.Client, userID int64) error {
	if client == nil {
		return nil
	}
	allowed, err := reserveAttemptScript.Run(ctx, client, []string{PasswordAttemptKey(userID)},
		MaxPasswordAttempts, PasswordAttemptWindow.Milliseconds()).Int64()
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "reserve password attempt")
	}
	if allowed == 0 {
		return xerr.Errorf(xerr.TooManyRequests, "too many failed password attempts, try again later")
	}
	return nil
}

// ClearPasswordAttempts forgets the attempts once the owner proves the
// password.
func ClearPasswordAttempts(ctx context.Context, client *redis.Client, userID int64) {
	if client == nil {
		return
	}
	if err := client.Del(ctx, PasswordAttemptKey(userID)).Err(); err != nil {
		logger.WithContext(ctx).Errorw("clear password attempts failed", logger.Field("error", err.Error()), logger.Field("user_id", userID))
	}
}
