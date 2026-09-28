// Package tasklock keeps two runs of one periodic task from overlapping,
// across replicas, with a Redis lock that only its owner may release.
package tasklock

import (
	"context"
	"time"

	"uuid"

	"github.com/redis/go-redis/v9"
)

// releaseScript deletes the lock only while it still holds the owner's token:
// a run that outlived its TTL must not free the lock of the run that took it
// over.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// Lock is a held task lock.
type Lock struct {
	client *redis.Client
	key    string
	token  string
}

// Acquire takes the lock at key for ttl. ok is false, with a nil error, when
// another run holds it.
func Acquire(ctx context.Context, client *redis.Client, key string, ttl time.Duration) (lock *Lock, ok bool, err error) {
	token := uuid.NewV7().String()
	ok, err = client.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return nil, false, err
	}
	return &Lock{client: client, key: key, token: token}, true, nil
}

// Release frees the lock if this run still owns it and reports whether it
// did; an expired lock taken over by another run is left alone.
func (l *Lock) Release(ctx context.Context) (bool, error) {
	// Release after the run's context ended (a cancelled task) still frees
	// the lock instead of leaving it to its TTL.
	deleted, err := releaseScript.Run(context.WithoutCancel(ctx), l.client, []string{l.key}, l.token).Int()
	return deleted == 1, err
}
