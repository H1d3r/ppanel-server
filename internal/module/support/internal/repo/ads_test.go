package repo

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/support/entity/ads"
	"github.com/perfect-panel/server/pkg/cache"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGetAdsListByPageKeepsOnlyScheduledAds(t *testing.T) {
	db := openSupportTestDB(t, &ads.Ads{})
	now := time.Now()
	unset := time.UnixMilli(0) // what the admin API stores for an omitted time
	for _, row := range []*ads.Ads{
		{Title: "running", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour), Status: 1},
		{Title: "scheduled", StartTime: now.Add(time.Hour), EndTime: now.Add(2 * time.Hour), Status: 1},
		{Title: "expired", StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Hour), Status: 1},
		{Title: "no end", StartTime: now.Add(-time.Hour), EndTime: unset, Status: 1},
		{Title: "no start", StartTime: unset, EndTime: now.Add(time.Hour), Status: 1},
		{Title: "no start, expired", StartTime: unset, EndTime: now.Add(-time.Hour), Status: 1},
		{Title: "unbounded", StartTime: unset, EndTime: unset, Status: 1},
		{Title: "zero bounds", Status: 1},
		{Title: "null bounds", Status: 1},
		{Title: "disabled", StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour), Status: 0},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("UPDATE ads SET start_time = NULL, end_time = NULL WHERE title = ?", "null bounds").Error; err != nil {
		t.Fatal(err)
	}

	enabled := 1
	total, list, err := NewAdsRepo(cache.NewConn(db, nil)).GetAdsListByPage(context.Background(), 1, 200, ads.Filter{Status: &enabled, ActiveAt: &now})
	if err != nil {
		t.Fatalf("GetAdsListByPage: %v", err)
	}
	var got []string
	for _, row := range list {
		got = append(got, row.Title)
	}
	sort.Strings(got)
	want := []string{"no end", "no start", "null bounds", "running", "unbounded", "zero bounds"}
	if total != int64(len(want)) || strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("active ads = %v (total %d), want %v", got, total, want)
	}
}

// The admin list applies no schedule: it must keep showing every ad.
func TestGetAdsListByPageWithoutActiveAtIgnoresSchedule(t *testing.T) {
	db := openSupportTestDB(t, &ads.Ads{})
	now := time.Now()
	for _, row := range []*ads.Ads{
		{Title: "scheduled", StartTime: now.Add(time.Hour), EndTime: now.Add(2 * time.Hour), Status: 1},
		{Title: "expired", StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Hour), Status: 1},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	total, _, err := NewAdsRepo(cache.NewConn(db, nil)).GetAdsListByPage(context.Background(), 1, 10, ads.Filter{})
	if err != nil || total != 2 {
		t.Fatalf("admin list total = %d (err %v), want both ads", total, err)
	}
}

func TestAdsActiveAtSQL(t *testing.T) {
	for name, dialector := range map[string]gorm.Dialector{
		"mysql": mysql.New(mysql.Config{
			DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=true&loc=Local",
			SkipInitializeWithVersion: true,
		}),
		"postgres": postgres.New(postgres.Config{
			DSN:                  "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable",
			PreferSimpleProtocol: true,
		}),
	} {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			if err != nil {
				t.Fatalf("open dry-run database: %v", err)
			}
			now := time.Now()
			var rows []*ads.Ads
			stmt := db.Model(&ads.Ads{}).Scopes(adsActiveAt(now)).Find(&rows).Statement
			sql := stmt.SQL.String()
			for _, want := range []string{"(start_time IS NULL OR start_time <= ", "(end_time IS NULL OR end_time <= ", " OR end_time > "} {
				if !strings.Contains(sql, want) {
					t.Fatalf("schedule SQL missing %q:\n%s", want, sql)
				}
			}
			if len(stmt.Vars) != 3 || !stmt.Vars[0].(time.Time).Equal(now) || !stmt.Vars[1].(time.Time).Equal(time.UnixMilli(0)) || !stmt.Vars[2].(time.Time).Equal(now) {
				t.Fatalf("schedule vars = %v, want now, the unset-bound epoch, now", stmt.Vars)
			}
		})
	}
}
