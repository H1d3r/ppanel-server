package repo

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSQLiteUserRepo(t *testing.T, name string) (*gorm.DB, *UserRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&user.User{}, &user.AuthMethods{}); err != nil {
		t.Fatal(err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	return db, NewUserRepo(repository.ModuleConn{DB: db, Redis: redisClient}.Conn(), repository.IdentityBridges{})
}

func TestFindEnabledUserIDsExcludesDeletedAndDisabledUsers(t *testing.T) {
	db, repo := newSQLiteUserRepo(t, "active-user-ids")

	enabled, disabled := true, false
	users := []*user.User{{Enable: &enabled}, {Enable: &disabled}, {Enable: &enabled}}
	for _, item := range users {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(users[2]).Error; err != nil {
		t.Fatal(err)
	}
	ids, err := repo.FindEnabledUserIDs(context.Background(), []int64{users[0].Id, users[1].Id, users[2].Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != users[0].Id {
		t.Fatalf("enabled ids = %v, want [%d]", ids, users[0].Id)
	}
}

// Deleting a user only soft-deletes the user row, so the bindings stay
// behind; the lookup behind subscription notices must not resolve them.
func TestFindUserAuthMethodsByUserIdsSkipsDeletedUsers(t *testing.T) {
	db, repo := newSQLiteUserRepo(t, "auth-methods-deleted-users")

	// The telegram binding makes binding ids differ from user ids, so a join
	// leaking user columns into the result would show.
	live := &user.User{AuthMethods: []user.AuthMethods{
		{AuthType: "telegram", AuthIdentifier: "10001"},
		{AuthType: "email", AuthIdentifier: "live@example.com"},
	}}
	deleted := &user.User{AuthMethods: []user.AuthMethods{
		{AuthType: "email", AuthIdentifier: "deleted@example.com"},
	}}
	for _, item := range []*user.User{live, deleted} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(deleted).Error; err != nil {
		t.Fatal(err)
	}

	methods, err := repo.FindUserAuthMethodsByUserIds(context.Background(), "email", []int64{live.Id, deleted.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 1 {
		t.Fatalf("methods = %d, want only the live user's email binding", len(methods))
	}
	want := live.AuthMethods[1]
	if got := methods[0]; got.Id != want.Id || got.UserId != live.Id || got.AuthIdentifier != want.AuthIdentifier {
		t.Fatalf("method = %+v, want %+v", got, want)
	}
}

func TestEmailRecipientsSkipDeletedUsers(t *testing.T) {
	db, repo := newSQLiteUserRepo(t, "email-recipients-deleted-users")

	live := &user.User{AuthMethods: []user.AuthMethods{{AuthType: "email", AuthIdentifier: "live@example.com"}}}
	deleted := &user.User{AuthMethods: []user.AuthMethods{{AuthType: "email", AuthIdentifier: "deleted@example.com"}}}
	for _, item := range []*user.User{live, deleted} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(deleted).Error; err != nil {
		t.Fatal(err)
	}

	filter := &user.EmailRecipientFilter{Scope: 1}
	emails, err := repo.QueryEmailRecipients(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 1 || emails[0] != "live@example.com" {
		t.Fatalf("recipients = %v, want [live@example.com]", emails)
	}
	count, err := repo.CountEmailRecipients(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recipient count = %d, want 1", count)
	}
}
