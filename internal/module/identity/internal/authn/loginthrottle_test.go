package authn

import (
	"context"
	"testing"

	"errors"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

// Password guesses against one account run out after maxLoginFailures; the
// lockout ends with its window, and the owner signing in clears it.
func TestLoginFailuresLockPasswordSignInForTheWindow(t *testing.T) {
	rdb := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: rdb.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	for i := 0; i < maxLoginFailures; i++ {
		if err := ensureLoginAllowed(ctx, client, 7); err != nil {
			t.Fatalf("attempt %d refused early: %v", i+1, err)
		}
		recordLoginFailure(ctx, client, 7)
	}
	err := ensureLoginAllowed(ctx, client, 7)
	var codeErr *xerr.CodeError
	if !errors.As(err, &codeErr) || codeErr.GetErrCode() != xerr.TooManyRequests {
		t.Fatalf("after %d failures: error = %v, want TooManyRequests", maxLoginFailures, err)
	}
	if ttl := rdb.TTL(loginFailureKey(7)); ttl <= 0 || ttl > loginFailureWindow {
		t.Fatalf("lockout TTL = %v, want within the %v window", ttl, loginFailureWindow)
	}
	if err := ensureLoginAllowed(ctx, client, 8); err != nil {
		t.Fatalf("another account was locked: %v", err)
	}

	rdb.FastForward(loginFailureWindow)
	if err := ensureLoginAllowed(ctx, client, 7); err != nil {
		t.Fatalf("lockout outlasted its window: %v", err)
	}

	recordLoginFailure(ctx, client, 7)
	clearLoginFailures(ctx, client, 7)
	if rdb.Exists(loginFailureKey(7)) {
		t.Fatal("a successful sign-in did not clear the failures")
	}
}
