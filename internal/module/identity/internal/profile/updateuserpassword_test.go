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
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

type passwordUsers struct {
	repository.UserRepo
	written map[string]interface{}
}

func (r *passwordUsers) UpdateColumns(_ context.Context, _ int64, columns map[string]interface{}) error {
	r.written = columns
	return nil
}

func newPasswordLogic(t *testing.T, current *usermodel.User) (*UpdateUserPasswordLogic, *passwordUsers, *redis.Client) {
	t.Helper()
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	users := &passwordUsers{}
	ctx := usermodel.NewContext(context.Background(), current)
	return newUpdateUserPasswordLogic(ctx, Deps{Users: users, Redis: rds}), users, rds
}

// A session alone must not be enough to change the password: that would turn
// a stolen session into the account for good.
func TestUpdateUserPasswordRequiresCurrentPasswordAndEndsSessions(t *testing.T) {
	current := &usermodel.User{Id: 7, Password: password.EncodePassWord("old-password"), Algo: password.PasswordAlgoArgon2id}
	logic, users, rds := newPasswordLogic(t, current)
	before, err := usersession.AcquireEpoch(context.Background(), rds, 7)
	if err != nil {
		t.Fatal(err)
	}

	if err := logic.UpdateUserPassword(&dto.UpdateUserPasswordRequest{OldPassword: "guessed", Password: "new-password-1"}); err == nil || users.written != nil {
		t.Fatalf("wrong current password: error = %v, written = %v", err, users.written)
	}

	if err := logic.UpdateUserPassword(&dto.UpdateUserPasswordRequest{OldPassword: "old-password", Password: "new-password-1"}); err != nil {
		t.Fatalf("UpdateUserPassword() error = %v", err)
	}
	if hash, _ := users.written["password"].(string); !password.VerifyPassWord("new-password-1", hash) {
		t.Fatal("new password was not written")
	}
	after, _ := rds.Get(context.Background(), usersession.Key(7)).Result()
	if usersession.Check(map[string]interface{}{usersession.EpochClaim: before}, after) == nil {
		t.Fatal("sessions from before the change still work")
	}
}

// Accounts created through OAuth or device sign-in have no password to prove.
func TestUpdateUserPasswordSetsFirstPasswordWithoutCurrentOne(t *testing.T) {
	logic, users, _ := newPasswordLogic(t, &usermodel.User{Id: 7})

	if err := logic.UpdateUserPassword(&dto.UpdateUserPasswordRequest{Password: "first-password"}); err != nil {
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

	if err := newLogoutLogic(ctx, Deps{Redis: rds}).Logout(); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if rds.Exists(context.Background(), usersession.SessionKey("current")).Val() != 0 {
		t.Fatal("the calling session still exists")
	}
	if rds.Exists(context.Background(), usersession.SessionKey("other")).Val() != 1 {
		t.Fatal("another session was ended")
	}
}
