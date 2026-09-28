package repo

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/perfect-panel/server/pkg/timeutil"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestServableSubscribesSQL pins the node user-list predicate on both
// production dialects: the status set plus the sweep's expiry and traffic
// conditions, with the epoch sentinel and "now" bound as parameters.
func TestServableSubscribesSQL(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		dialector gorm.Dialector
		want      []string
	}{
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
				SkipInitializeWithVersion: true,
			}),
			want: []string{
				"FROM `user_subscribe`",
				"subscribe_id IN (?,?) AND status IN (?,?)",
				"(expire_time IS NULL OR expire_time = ? OR expire_time > ?)",
				"(traffic > 0 AND upload + download >= traffic) IS NOT TRUE",
				"ORDER BY subscribe_id ASC, id ASC",
			},
		},
		{
			name: "postgres",
			dialector: postgres.New(postgres.Config{
				DSN:                  "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable",
				PreferSimpleProtocol: true,
			}),
			want: []string{
				`FROM "user_subscribe"`,
				"subscribe_id IN ($1,$2) AND status IN ($3,$4)",
				"(expire_time IS NULL OR expire_time = $5 OR expire_time > $6)",
				"(traffic > 0 AND upload + download >= traffic) IS NOT TRUE",
				"ORDER BY subscribe_id ASC, id ASC",
			},
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
				t.Fatalf("open dry-run database: %v", err)
			}

			var rows []*usersub.Subscribe
			stmt := servableSubscribes(db, []int64{3, 4}, now).Find(&rows).Statement
			sql := stmt.SQL.String()
			for _, want := range tt.want {
				if !strings.Contains(sql, want) {
					t.Fatalf("SQL missing %q:\n%s", want, sql)
				}
			}
			if len(stmt.Vars) != 6 {
				t.Fatalf("SQL has %d parameters, want 6: %s", len(stmt.Vars), sql)
			}
			if got, ok := stmt.Vars[4].(time.Time); !ok || !got.Equal(time.UnixMilli(0)) {
				t.Fatalf("sentinel parameter = %#v, want the epoch", stmt.Vars[4])
			}
			if got, ok := stmt.Vars[5].(time.Time); !ok || !got.Equal(now) {
				t.Fatalf("now parameter = %#v, want %v", stmt.Vars[5], now)
			}
		})
	}
}

// TestFindUsersSubscribeBySubscribeIdsDropsExpiredAndExhausted runs the
// query: rows the lifecycle sweep is due to finish lose node access before
// the sweep flips their status, while the no-limit sentinel, a NULL expiry
// and unlimited or remaining traffic keep it.
func TestFindUsersSubscribeBySubscribeIdsDropsExpiredAndExhausted(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// The model carries MySQL-specific type/default tags; use the portable
	// equivalent of the production table.
	if err := db.Exec(`CREATE TABLE user_subscribe (
 id INTEGER PRIMARY KEY, user_id BIGINT, order_id BIGINT, subscribe_id BIGINT,
 start_time TIMESTAMP, expire_time TIMESTAMP, finished_at TIMESTAMP,
 traffic BIGINT DEFAULT 0, download BIGINT DEFAULT 0, upload BIGINT DEFAULT 0,
 token VARCHAR(255) UNIQUE, uuid VARCHAR(255) UNIQUE, status INTEGER DEFAULT 0,
 note TEXT, entitlement_source VARCHAR(32) NOT NULL DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	now := timeutil.Now()
	future, past := now.Add(time.Hour), now.Add(-time.Minute)
	rows := []usersub.Subscribe{
		{Id: 1, SubscribeId: 7, Status: usersub.SubscribeStatusActive, ExpireTime: future, Traffic: 100, Upload: 10, Download: 10},
		{Id: 2, SubscribeId: 7, Status: usersub.SubscribeStatusActive, ExpireTime: past},
		{Id: 3, SubscribeId: 7, Status: usersub.SubscribeStatusActive, ExpireTime: time.UnixMilli(0), Upload: 1 << 40, Download: 1},
		{Id: 4, SubscribeId: 7, Status: usersub.SubscribeStatusPending, ExpireTime: future, Traffic: 100, Upload: 60, Download: 40},
		{Id: 5, SubscribeId: 7, Status: usersub.SubscribeStatusStopped, ExpireTime: future},
		{Id: 6, SubscribeId: 8, Status: usersub.SubscribeStatusActive, ExpireTime: future},
		{Id: 7, SubscribeId: 7, Status: usersub.SubscribeStatusActive, ExpireTime: future, Traffic: 100, Upload: 99},
		{Id: 8, SubscribeId: 7, Status: usersub.SubscribeStatusActive, ExpireTime: past},
		{Id: 9, SubscribeId: 7, Status: usersub.SubscribeStatusPending, ExpireTime: time.UnixMilli(0), Traffic: 100, Upload: 100},
	}
	for i := range rows {
		rows[i].Token = fmt.Sprintf("token-%d", rows[i].Id)
		rows[i].UUID = fmt.Sprintf("uuid-%d", rows[i].Id)
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	// A NULL expiry never expires in the sweep either.
	if err := db.Exec("UPDATE user_subscribe SET expire_time = NULL WHERE id = 8").Error; err != nil {
		t.Fatal(err)
	}

	got, err := NewUserSubscriptionRepo(cache.NewConn(db, nil)).FindUsersSubscribeBySubscribeIds(context.Background(), []int64{7})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(got))
	for _, sub := range got {
		ids = append(ids, sub.Id)
	}
	if fmt.Sprint(ids) != "[1 3 7 8]" {
		t.Fatalf("served subscriptions = %v, want [1 3 7 8]", ids)
	}
}
