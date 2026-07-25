package repo

import (
	"bytes"
	"context"
	"github.com/perfect-panel/server/internal/repository"
	"log"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func ptr[T any](v T) *T {
	return &v
}

func TestUserRepoFindOneForUpdateUsesRowLockAndDefaultScope(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 gormlogger.New(log.New(&logs, "", 0), gormlogger.Config{LogLevel: gormlogger.Info}),
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	if _, err := NewUserRepo(repository.ModuleConn{DB: db, Redis: redisClient}.Conn(), nil).FindOneForUpdate(context.Background(), 42); err != nil {
		t.Fatalf("FindOneForUpdate: %v", err)
	}
	sql := logs.String()
	for _, want := range []string{"FROM `user`", "WHERE id = 42", "`user`.`deleted_at` IS NULL", "FOR UPDATE"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestFindDeviceOnlineRecordUsesCreatedAt(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               gormlogger.New(log.New(&logs, "", 0), gormlogger.Config{LogLevel: gormlogger.Info}),
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	_, err = NewUserRepo(repository.ModuleConn{DB: db, Redis: redisClient}.Conn(), nil).FindDeviceOnlineRecord(context.Background(), 42, "2026-07-21 00:00:00", "2026-07-22 00:00:00")
	if err != nil {
		t.Fatalf("FindDeviceOnlineRecord: %v", err)
	}
	sql := logs.String()
	if !strings.Contains(sql, "created_at >=") || strings.Contains(sql, "create_at") {
		t.Fatalf("expected created_at time predicate, got:\n%s", sql)
	}
}

func TestApplyUserPageFiltersSearchSQL(t *testing.T) {
	tests := []struct {
		name       string
		dialector  gorm.Dialector
		wantSQL    []string
		wantNoSQL  []string
		wantSearch string
	}{
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
				SkipInitializeWithVersion: true,
			}),
			wantSQL: []string{
				"FROM `user`",
				"`user`.`id` = ?",
				"`user`.`refer_code` LIKE ? ESCAPE '='",
				"EXISTS (SELECT 1 FROM `user_auth_methods`",
				"`user_auth_methods`.`user_id` = `user`.`id`",
				"`user_auth_methods`.`auth_identifier` LIKE ? ESCAPE '='",
				"EXISTS (SELECT 1 FROM `user_subscribe`",
				"`user_subscribe`.`user_id` = `user`.`id`",
				"`user_subscribe`.`id` = ?",
				"`user_subscribe`.`subscribe_id` = ?",
				"`user_subscribe`.`status` IN (?,?)",
				"ORDER BY `user`.`id` DESC",
			},
			wantNoSQL:  []string{"LEFT JOIN", "GROUP BY"},
			wantSearch: "alice=_100=%@example.com%",
		},
		{
			name: "postgres",
			dialector: postgres.New(postgres.Config{
				DSN:                  "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable",
				PreferSimpleProtocol: true,
			}),
			wantSQL: []string{
				`FROM "user"`,
				`"user"."id" = $1`,
				`"user"."refer_code" LIKE $2 ESCAPE '='`,
				`EXISTS (SELECT 1 FROM "user_auth_methods"`,
				`"user_auth_methods"."user_id" = "user"."id"`,
				`"user_auth_methods"."auth_identifier" LIKE $3 ESCAPE '='`,
				`EXISTS (SELECT 1 FROM "user_subscribe"`,
				`"user_subscribe"."user_id" = "user"."id"`,
				`"user_subscribe"."id" = $4`,
				`"user_subscribe"."subscribe_id" = $5`,
				`"user_subscribe"."status" IN ($6,$7)`,
				`ORDER BY "user"."id" DESC`,
			},
			wantNoSQL:  []string{"LEFT JOIN", "GROUP BY"},
			wantSearch: "alice=_100=%@example.com%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(tt.dialector, &gorm.Config{
				DryRun:                 true,
				DisableAutomaticPing:   true,
				SkipDefaultTransaction: true,
			})
			if err != nil {
				t.Fatalf("open gorm db: %v", err)
			}

			var result []user.User
			filter := &user.UserFilterParams{
				Search:          "alice_100%@example.com",
				UserId:          ptr[int64](99),
				UserSubscribeId: ptr[int64](10),
				SubscribeId:     ptr[int64](20),
				Order:           "DESC",
			}
			stmt := applyUserPageFilters(db.Model(&user.User{}), filter).Find(&result).Statement
			sql := stmt.SQL.String()

			for _, want := range tt.wantSQL {
				if !strings.Contains(sql, want) {
					t.Fatalf("SQL missing %q:\n%s", want, sql)
				}
			}
			for _, unwanted := range tt.wantNoSQL {
				if strings.Contains(sql, unwanted) {
					t.Fatalf("SQL should not contain %q:\n%s", unwanted, sql)
				}
			}
			subscribeFrom := "FROM `user_subscribe`"
			if tt.name == "postgres" {
				subscribeFrom = `FROM "user_subscribe"`
			}
			if got := strings.Count(sql, subscribeFrom); got != 1 {
				t.Fatalf("subscribe filters should use one user_subscribe EXISTS, got %d:\n%s", got, sql)
			}
			if got := stmt.Vars[1]; got != tt.wantSearch {
				t.Fatalf("refer_code search pattern = %#v, want %#v", got, tt.wantSearch)
			}
			if got := stmt.Vars[2]; got != tt.wantSearch {
				t.Fatalf("auth search pattern = %#v, want %#v", got, tt.wantSearch)
			}
		})
	}
}

func TestApplyUserPageFiltersSkipsBlankSearch(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	var result []user.User
	stmt := applyUserPageFilters(db.Model(&user.User{}), &user.UserFilterParams{Search: "   "}).Find(&result).Statement
	sql := stmt.SQL.String()
	if strings.Contains(sql, "LIKE") || strings.Contains(sql, "user_auth_methods") {
		t.Fatalf("blank search should not add search filters:\n%s", sql)
	}
	if len(stmt.Vars) != 0 {
		t.Fatalf("vars len = %d, want 0: %#v", len(stmt.Vars), stmt.Vars)
	}
}

func TestApplyUserPageFiltersMatchesSubscribeTokenOrUUID(t *testing.T) {
	tests := []struct {
		name      string
		dialector gorm.Dialector
		wantSQL   []string
	}{
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
				SkipInitializeWithVersion: true,
			}),
			wantSQL: []string{
				"EXISTS (SELECT 1 FROM `user_subscribe`",
				"`user_subscribe`.`user_id` = `user`.`id`",
				"(`user_subscribe`.`token` = ? OR `user_subscribe`.`uuid` = ?)",
			},
		},
		{
			name: "postgres",
			dialector: postgres.New(postgres.Config{
				DSN:                  "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable",
				PreferSimpleProtocol: true,
			}),
			wantSQL: []string{
				`EXISTS (SELECT 1 FROM "user_subscribe"`,
				`"user_subscribe"."user_id" = "user"."id"`,
				`("user_subscribe"."token" = $1 OR "user_subscribe"."uuid" = $2)`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(tt.dialector, &gorm.Config{
				DryRun:               true,
				DisableAutomaticPing: true,
			})
			if err != nil {
				t.Fatalf("open gorm db: %v", err)
			}

			var result []user.User
			stmt := applyUserPageFilters(db.Model(&user.User{}), &user.UserFilterParams{
				UserSubscribeToken: "sub-token-or-uuid",
			}).Find(&result).Statement
			sql := stmt.SQL.String()

			for _, want := range tt.wantSQL {
				if !strings.Contains(sql, want) {
					t.Fatalf("SQL missing %q:\n%s", want, sql)
				}
			}
			if strings.Contains(sql, "status") {
				t.Fatalf("token/uuid lookup should not add status filter:\n%s", sql)
			}
			if len(stmt.Vars) != 2 {
				t.Fatalf("vars len = %d, want 2: %#v", len(stmt.Vars), stmt.Vars)
			}
			for index, got := range stmt.Vars {
				if got != "sub-token-or-uuid" {
					t.Fatalf("var[%d] = %#v, want subscribe token", index, got)
				}
			}
		})
	}
}

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		size     int
		wantPage int
		wantSize int
	}{
		{name: "zero values use safe defaults", page: 0, size: 0, wantPage: 1, wantSize: repository.DefaultPageSize},
		{name: "negative values use safe defaults", page: -2, size: -10, wantPage: 1, wantSize: repository.DefaultPageSize},
		{name: "large size is capped", page: 2, size: repository.MaxPageSize + 1, wantPage: 2, wantSize: repository.MaxPageSize},
		{name: "valid values pass through", page: 3, size: 50, wantPage: 3, wantSize: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPage, gotSize := repository.NormalizePage(tt.page, tt.size)
			if gotPage != tt.wantPage || gotSize != tt.wantSize {
				t.Fatalf("repository.NormalizePage(%d, %d) = (%d, %d), want (%d, %d)",
					tt.page, tt.size, gotPage, gotSize, tt.wantPage, tt.wantSize)
			}
		})
	}
}

func TestNormalizePageFloor(t *testing.T) {
	gotPage, gotSize := repository.NormalizePageFloor(0, repository.MaxPageSize+1)
	if gotPage != 1 || gotSize != repository.MaxPageSize+1 {
		t.Fatalf("repository.NormalizePageFloor() = (%d, %d), want (1, %d)", gotPage, gotSize, repository.MaxPageSize+1)
	}
}
