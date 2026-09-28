package oauthstate

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

// A state is redeemed once, only by the method it was issued for, and not
// after it expired.
func TestIssuedStateIsRedeemedOnce(t *testing.T) {
	server, client := newClient(t)
	ctx := context.Background()

	state, err := Issue(ctx, client, "google", "https://panel.example/callback")
	if err != nil || state == "" {
		t.Fatalf("Issue() = %q, %v", state, err)
	}
	if ttl := server.TTL(key("google", state)); ttl != TTL {
		t.Fatalf("state TTL = %v, want %v", ttl, TTL)
	}
	if _, err := Consume(ctx, client, "github", state); !errors.Is(err, ErrUnknown) {
		t.Fatalf("another method redeemed the state: %v", err)
	}
	peeked, err := Peek(ctx, client, "google", state)
	if err != nil || peeked != "https://panel.example/callback" {
		t.Fatalf("Peek() = %q, %v", peeked, err)
	}
	redirect, err := Consume(ctx, client, "google", state)
	if err != nil || redirect != "https://panel.example/callback" {
		t.Fatalf("Consume() = %q, %v", redirect, err)
	}
	if _, err := Consume(ctx, client, "google", state); !errors.Is(err, ErrUnknown) {
		t.Fatalf("a redeemed state was redeemed again: %v", err)
	}

	expiring, err := Issue(ctx, client, "apple", "https://panel.example/callback")
	if err != nil {
		t.Fatal(err)
	}
	server.FastForward(TTL)
	if _, err := Peek(ctx, client, "apple", expiring); !errors.Is(err, ErrUnknown) {
		t.Fatalf("an expired state was found: %v", err)
	}
}

func TestIssuedStatesAreDistinct(t *testing.T) {
	_, client := newClient(t)
	first, err := Issue(context.Background(), client, "google", "https://panel.example/a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Issue(context.Background(), client, "google", "https://panel.example/b")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two sign-ins got the same state")
	}
}
