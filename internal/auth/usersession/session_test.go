package usersession

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRevokeInvalidatesEarlierSessionsOnly(t *testing.T) {
	rdb := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: rdb.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	legacy := map[string]interface{}{"UserId": float64(1)}
	if err := Check(legacy, ""); err != nil {
		t.Fatalf("token from before epochs rejected for a never-revoked user: %v", err)
	}

	first, err := AcquireEpoch(ctx, client, 1)
	if err != nil || first == "" {
		t.Fatalf("AcquireEpoch() = %q, %v", first, err)
	}
	if again, _ := AcquireEpoch(ctx, client, 1); again != first {
		t.Fatalf("second session got epoch %q, want the shared %q", again, first)
	}
	issued := map[string]interface{}{EpochClaim: first}
	if err := Check(issued, first); err != nil {
		t.Fatalf("current session rejected: %v", err)
	}
	if err := Check(legacy, first); err != nil {
		t.Fatalf("legacy token rejected before any revocation: %v", err)
	}

	if err := Revoke(ctx, client, 1); err != nil {
		t.Fatal(err)
	}
	current, _ := client.Get(ctx, Key(1)).Result()
	if rdb.TTL(Key(1)) != 0 {
		t.Fatal("epoch must not expire")
	}
	if Check(issued, current) == nil || Check(legacy, current) == nil {
		t.Fatal("a session issued before the revocation is still valid")
	}
	next, _ := AcquireEpoch(ctx, client, 1)
	if next != current || Check(map[string]interface{}{EpochClaim: next}, current) != nil {
		t.Fatal("a session issued after the revocation is not valid")
	}
	if other, _ := AcquireEpoch(ctx, client, 2); other == current {
		t.Fatal("users share an epoch")
	}

	// An evicted epoch fails tokens that carry one closed.
	rdb.Del(Key(1))
	if Check(map[string]interface{}{EpochClaim: next}, "") == nil {
		t.Fatal("token accepted after its epoch was evicted")
	}
}
