package profile

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	usermodel "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/redis/go-redis/v9"
)

// passwordUsers records the columns the flow writes.
type passwordUsers struct {
	written map[string]any
}

var _ Users = (*passwordUsers)(nil)

func (r *passwordUsers) UpdateColumns(_ context.Context, _ int64, columns map[string]any) error {
	r.written = columns
	return nil
}

// newPasswordService returns the service, its account rows and Redis, and
// the context of the signed-in account current.
func newPasswordService(t *testing.T, current *usermodel.User) (*Service, *passwordUsers, *redis.Client, context.Context) {
	t.Helper()
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	users := &passwordUsers{}
	ctx := usermodel.NewContext(context.Background(), current)
	return NewService(Deps{Users: users, Redis: rds}), users, rds, ctx
}

// A session alone must not be enough to change the password: that would turn
// a stolen session into the account for good.
func TestUpdateUserPasswordRequiresCurrentPasswordAndEndsSessions(t *testing.T) {
	current := &usermodel.User{Id: 7, Password: password.EncodePassWord("old-password"), Algo: password.PasswordAlgoArgon2id}
	svc, users, rds, ctx := newPasswordService(t, current)
	before, err := usersession.AcquireEpoch(context.Background(), rds, 7)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateUserPassword(ctx, &dto.UpdateUserPasswordRequest{OldPassword: "guessed", Password: "new-password-1"}); err == nil || users.written != nil {
		t.Fatalf("wrong current password: error = %v, written = %v", err, users.written)
	}

	if err := svc.UpdateUserPassword(ctx, &dto.UpdateUserPasswordRequest{OldPassword: "old-password", Password: "new-password-1"}); err != nil {
		t.Fatalf("UpdateUserPassword() error = %v", err)
	}
	if hash, _ := users.written["password"].(string); !password.VerifyPassWord("new-password-1", hash) {
		t.Fatal("new password was not written")
	}
	after, _ := rds.Get(context.Background(), usersession.Key(7)).Result()
	if usersession.Check(map[string]any{usersession.EpochClaim: before}, after) == nil {
		t.Fatal("sessions from before the change still work")
	}
}

// Accounts created through OAuth or device sign-in have no password to prove.
func TestUpdateUserPasswordSetsFirstPasswordWithoutCurrentOne(t *testing.T) {
	svc, users, _, ctx := newPasswordService(t, &usermodel.User{Id: 7})

	if err := svc.UpdateUserPassword(ctx, &dto.UpdateUserPasswordRequest{Password: "first-password"}); err != nil {
		t.Fatalf("UpdateUserPassword() error = %v", err)
	}
	if users.written["password"] == nil {
		t.Fatal("first password was not written")
	}
}

func TestLogoutEndsOnlyTheCallingSession(t *testing.T) {
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	for _, id := range []string{"current", "other"} {
		if err := rds.Set(context.Background(), usersession.SessionKey(id), 7, 0).Err(); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeySessionID, "current")

	if err := NewService(Deps{Redis: rds}).Logout(ctx); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if rds.Exists(context.Background(), usersession.SessionKey("current")).Val() != 0 {
		t.Fatal("the calling session still exists")
	}
	if rds.Exists(context.Background(), usersession.SessionKey("other")).Val() != 1 {
		t.Fatal("another session was ended")
	}
}
