package routes

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

const adminGuardJWTKey = "admin-guard-test-only-signing-key"

// Every admin route group is opened through adminGroup, which installs the
// administrator guard. A group opened directly would serve its routes to any
// signed-in user.
func TestAdminGroupsAreGuarded(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), `Group("/v1/admin`) {
			t.Errorf("%s opens an admin route group directly; use deps.adminGroup", name)
		}
	}
}

// A signed-in user who is not an administrator is refused on an admin route;
// an administrator gets through.
func TestAdminGroupRefusesNonAdministrators(t *testing.T) {
	logtest.Discard(t)
	store, rds := adminGuardStore(t)
	enabled, isAdmin := true, true
	member := &user.User{Enable: &enabled}
	admin := &user.User{Enable: &enabled, IsAdmin: &isAdmin}
	for _, u := range []*user.User{member, admin} {
		if err := store.User().Insert(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}

	h := server.New()
	accounts := identity.New(identity.Deps{
		Store: store, Redis: rds, Users: store.User(), UserAuths: store.UserAuth(), Devices: store.UserDevice(),
		Cache: store.UserCache(), Logs: store.Log(), Auths: store.Auth(),
	})
	deps := Dependencies{Config: config.Config{Boot: config.Boot{JwtAuth: config.JwtAuth{AccessSecret: adminGuardJWTKey, AccessExpire: 3600}}}, Redis: rds, Identity: accounts}
	deps.adminGroup(h, "/v1/admin/probe").GET("/", func(_ context.Context, c *app.RequestContext) {
		c.String(200, "served")
	})

	for _, tc := range []struct {
		name       string
		user       *user.User
		wantServed bool
	}{
		{"member", member, false},
		{"administrator", admin, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signed, err := usersession.Issue(context.Background(), rds, adminGuardJWTKey, 3600, usersession.Grant{UserID: tc.user.Id})
			if err != nil {
				t.Fatal(err)
			}
			w := ut.PerformRequest(h.Engine, "GET", "/v1/admin/probe/", nil, ut.Header{Key: "Authorization", Value: signed})
			body := w.Body.String()
			if served := body == "served"; served != tc.wantServed {
				t.Fatalf("served = %v, want %v (body %q)", served, tc.wantServed, body)
			}
			if !tc.wantServed {
				var reply struct {
					Code uint32 `json:"code"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Code != xerr.InvalidAccess {
					t.Fatalf("refusal = %q, want code %s", body, strconv.Itoa(int(xerr.InvalidAccess)))
				}
			}
		})
	}
}

func adminGuardStore(t *testing.T) (*repository.GormStore, *redis.Client) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true, IgnoreRelationshipsWhenMigrating: true, Logger: gormlog.Default.LogMode(gormlog.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&user.User{}, &user.AuthMethods{}); err != nil {
		t.Fatal(err)
	}
	// SQLite index names are database-global, unlike the MySQL entity tags.
	if err := db.Migrator().RenameIndex(&user.AuthMethods{}, "idx_user_id", "idx_auth_methods_user_id"); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&user.Device{}); err != nil {
		t.Fatal(err)
	}
	mr := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rds.Close() })
	store := repository.NewGormStoreWithBuilders(db, rds, repository.Builders{
		Identity: identity.NewRepoBuilder(), Platform: platform.NewRepoBuilder(),
		Billing: func(repository.ModuleConn) repository.BillingRepos { return repository.BillingRepos{} },
		Network: func(repository.ModuleConn) repository.NetworkRepos { return repository.NetworkRepos{} },
		Subscription: func(repository.ModuleConn, repository.NodeCacheKeyBridge) repository.SubscriptionRepos {
			return repository.SubscriptionRepos{}
		},
		Support:      func(repository.ModuleConn) repository.SupportRepos { return repository.SupportRepos{} },
		Notification: func(repository.ModuleConn) repository.NotificationRepos { return repository.NotificationRepos{} },
	})
	return store, rds
}
