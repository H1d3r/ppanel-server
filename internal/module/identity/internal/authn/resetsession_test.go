package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	token2 "github.com/perfect-panel/server/internal/auth/token"
	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type resetUsers struct {
	repository.UserRepo
	user    *user.User
	written map[string]interface{}
}

func (r *resetUsers) FindOne(context.Context, int64) (*user.User, error) { return r.user, nil }
func (r *resetUsers) UpdateColumns(_ context.Context, _ int64, columns map[string]interface{}, _ ...*gorm.DB) error {
	r.written = columns
	return nil
}

type resetAuths struct{ repository.UserAuthRepo }

func (resetAuths) FindUserAuthMethodByOpenID(context.Context, string, string) (*user.AuthMethods, error) {
	return &user.AuthMethods{UserId: 5, AuthType: "email", AuthIdentifier: "owner@example.com", Verified: true}, nil
}

type resetLogs struct{ repository.LogRepo }

func (resetLogs) Insert(context.Context, *log.SystemLog) error { return nil }

type resetStore struct {
	users *resetUsers
}

func (s resetStore) User() repository.UserRepo         { return s.users }
func (s resetStore) UserAuth() repository.UserAuthRepo { return resetAuths{} }
func (s resetStore) Log() repository.LogRepo           { return resetLogs{} }

const resetSecret = "reset-test-secret"

func newResetFixture(t *testing.T, account *user.User) (*ResetPasswordLogic, *resetUsers, *redis.Client) {
	t.Helper()
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	key := fmt.Sprintf("%s:%s:%s", config.AuthCodeCacheKey, auth.Security, "owner@example.com")
	if err := verification.SaveVerificationCode(context.Background(), rds, key, "123456", time.Minute); err != nil {
		t.Fatal(err)
	}
	users := &resetUsers{user: account}
	logic := NewResetPasswordLogic(context.Background(), ResetPasswordDependencies{
		Store:  resetStore{users: users},
		Redis:  rds,
		Config: ResetPasswordConfig{JWTAccessSecret: resetSecret, JWTAccessExpire: 3600},
		Policy: &fakeEmailPasswordResetPolicy{},
	})
	return logic, users, rds
}

// sessionIsLive mirrors the auth middleware's session and epoch checks.
func sessionIsLive(t *testing.T, rds *redis.Client, token string) bool {
	t.Helper()
	claims, err := token2.ParseJwtToken(token, resetSecret)
	if err != nil {
		t.Fatal(err)
	}
	userID := int64(claims["UserId"].(float64))
	values, err := rds.MGet(context.Background(), fmt.Sprintf("%v:%v", config.SessionIdKey, claims["SessionId"]), usersession.Key(userID)).Result()
	if err != nil {
		t.Fatal(err)
	}
	session, _ := values[0].(string)
	epoch, _ := values[1].(string)
	return session == fmt.Sprint(userID) && usersession.Check(claims, epoch) == nil
}

// A reset usually follows a compromise, so sessions issued before it end
// while the one it issues works.
func TestResetPasswordRevokesEarlierSessions(t *testing.T) {
	enabled := true
	logic, users, rds := newResetFixture(t, &user.User{Id: 5, Enable: &enabled})
	earlier, err := issueLoginSession(context.Background(), rds, resetSecret, 3600, 5, "email", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := logic.ResetPassword(&dto.ResetPasswordRequest{Email: "owner@example.com", Code: "123456", Password: "new-password-1"})
	if err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}

	if users.written["password"] == nil {
		t.Fatal("password was not written")
	}
	if sessionIsLive(t, rds, earlier.Token) {
		t.Fatal("a session from before the reset still works")
	}
	if !sessionIsLive(t, rds, resp.Token) {
		t.Fatal("the session issued by the reset does not work")
	}
}

func TestResetPasswordRejectsDeletedAndDisabledAccounts(t *testing.T) {
	enabled, disabled := true, false
	deleted := &user.User{Id: 5, Enable: &enabled, DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}}
	for name, account := range map[string]*user.User{"deleted": deleted, "disabled": {Id: 5, Enable: &disabled}} {
		logic, users, _ := newResetFixture(t, account)
		if _, err := logic.ResetPassword(&dto.ResetPasswordRequest{Email: "owner@example.com", Code: "123456", Password: "new-password-1"}); err == nil {
			t.Fatalf("%s account was reset", name)
		}
		if users.written != nil {
			t.Fatalf("%s account's password was written", name)
		}
	}
}
