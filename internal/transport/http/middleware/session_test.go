package middleware

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	token2 "github.com/perfect-panel/server/internal/auth/token"
	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

const sessionTestSecret = "session-test-secret"

type sessionUsers struct{ repository.UserRepo }

func (sessionUsers) FindOne(_ context.Context, id int64) (*user.User, error) {
	enabled := true
	return &user.User{Id: id, Enable: &enabled}, nil
}

type sessionStore struct{ repository.Store }

func (sessionStore) User() repository.UserRepo { return sessionUsers{} }

func newSessionDeps(t *testing.T) (AuthDeps, *redis.Client) {
	t.Helper()
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	return AuthDeps{JWT: config.JwtAuth{AccessSecret: sessionTestSecret}, Redis: rds, Store: sessionStore{}}, rds
}

func sessionToken(t *testing.T, rds *redis.Client, userID int64, sessionID string) string {
	t.Helper()
	epoch, err := usersession.AcquireEpoch(context.Background(), rds, userID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := token2.NewJwtToken(sessionTestSecret, time.Now().Unix(), 3600,
		token2.WithOption("UserId", userID), token2.WithOption("SessionId", sessionID), token2.WithOption(usersession.EpochClaim, epoch))
	if err != nil {
		t.Fatal(err)
	}
	if err := rds.Set(context.Background(), fmt.Sprintf("%v:%v", config.SessionIdKey, sessionID), userID, 0).Err(); err != nil {
		t.Fatal(err)
	}
	return token
}

// Order event tickets are signed with the same secret but are not sessions;
// they must be refused rather than crash the request.
func TestAuthenticateRequestRefusesTokensThatAreNotSessions(t *testing.T) {
	deps, _ := newSessionDeps(t)
	ticket, err := token2.NewJwtToken(sessionTestSecret, time.Now().Unix(), 3600,
		token2.WithOption("OrderNo", "20260927000000"), token2.WithOption("Scope", "order_events"), token2.WithOption("UserId", int64(7)))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := AuthenticateRequest(context.Background(), deps, ticket, "/v1/public/user/info"); err == nil {
		t.Fatal("an order event ticket authenticated as a session")
	}
}

func TestAuthenticateRequestRefusesRevokedSessions(t *testing.T) {
	deps, rds := newSessionDeps(t)
	token := sessionToken(t, rds, 7, "session-1")
	if _, err := AuthenticateRequest(context.Background(), deps, token, "/v1/public/user/info"); err != nil {
		t.Fatalf("live session refused: %v", err)
	}

	if err := usersession.Revoke(context.Background(), rds, 7); err != nil {
		t.Fatal(err)
	}

	if _, err := AuthenticateRequest(context.Background(), deps, token, "/v1/public/user/info"); err == nil {
		t.Fatal("a revoked session still authenticates")
	}
	if _, err := AuthenticateRequest(context.Background(), deps, sessionToken(t, rds, 7, "session-2"), "/v1/public/user/info"); err != nil {
		t.Fatalf("session issued after the revocation refused: %v", err)
	}
}
