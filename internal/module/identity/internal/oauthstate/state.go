// Package oauthstate owns the OAuth state round trip: the state issued with
// an authorization URL, stored with the redirect it belongs to, and redeemed
// once by the callback. It also pins redirects to the site host and makes
// signed callbacks without a state single use.
package oauthstate

import (
	"context"
	"errors"
	"time"

	"github.com/perfect-panel/server/pkg/random"
	"github.com/redis/go-redis/v9"
)

// TTL bounds how long a sign-in may take between the authorization URL and
// its callback.
const TTL = 5 * time.Minute

// ErrUnknown reports a state that was never issued, was already redeemed or
// expired.
var ErrUnknown = errors.New("oauth state is unknown, used or expired")

var consumeScript = redis.NewScript(`
local value = redis.call("GET", KEYS[1])
if not value then
  return false
end
redis.call("DEL", KEYS[1])
return value
`)

// key is where the state of a sign-in through method is stored.
func key(method, state string) string { return method + ":" + state }

// Issue creates the state of a sign-in through method that comes back to
// redirect, and returns it for the authorization URL.
func Issue(ctx context.Context, client *redis.Client, method, redirect string) (string, error) {
	state := random.KeyNew(32, 1)
	if err := client.Set(ctx, key(method, state), redirect, TTL).Err(); err != nil {
		return "", err
	}
	return state, nil
}

// Consume redeems a state of method once and returns its redirect. The Lua
// implementation keeps it atomic on Redis versions older than 6.2.
func Consume(ctx context.Context, client *redis.Client, method, state string) (string, error) {
	redirect, err := consumeScript.Run(ctx, client, []string{key(method, state)}).Text()
	if errors.Is(err, redis.Nil) {
		return "", ErrUnknown
	}
	return redirect, err
}

// Peek returns the redirect of a state of method without redeeming it: the
// Apple form-post callback hands the state on to the sign-in, which redeems
// it.
func Peek(ctx context.Context, client *redis.Client, method, state string) (string, error) {
	redirect, err := client.Get(ctx, key(method, state)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrUnknown
	}
	return redirect, err
}
