package usersub

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStatusSet(t *testing.T) {
	if !LiveStatuses.Contains(SubscribeStatusActive) || LiveStatuses.Contains(SubscribeStatusFinished) {
		t.Fatalf("LiveStatuses membership is wrong: %v", LiveStatuses)
	}
	if got := HeldStatuses.Values(); !reflect.DeepEqual(got, []int64{4, 5}) {
		t.Fatalf("HeldStatuses.Values() = %v", got)
	}
	for status := uint8(0); status <= SubscribeStatusStopped; status++ {
		if !AllStatuses.Contains(status) {
			t.Fatalf("AllStatuses lacks %d", status)
		}
		if OnHold(status) != (status == SubscribeStatusDeducted || status == SubscribeStatusStopped) {
			t.Fatalf("OnHold(%d) is wrong", status)
		}
	}
}

func TestNoExpiry(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	for name, tt := range map[string]struct {
		at   time.Time
		want bool
	}{
		"NULL reads as the zero time": {time.Time{}, true},
		"epoch sentinel":              {NoLimitExpiry, true},
		"epoch in another zone":       {time.UnixMilli(0).In(shanghai), true},
		"a real expiry":               {time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false},
		"one millisecond after epoch": {time.UnixMilli(1), false},
	} {
		if got := NoExpiry(tt.at); got != tt.want {
			t.Fatalf("%s: NoExpiry = %v, want %v", name, got, tt.want)
		}
	}
}

// availabilityCase is one subscription row of the availability table; both
// the Go predicate and the SQL condition are checked against it.
type availabilityCase struct {
	name   string
	status uint8
	expire time.Time
	nullEx bool
	used   [2]int64 // upload, download
	quota  int64
	want   Availability
}

func availabilityCases(now time.Time) []availabilityCase {
	future, past := now.Add(time.Hour), now.Add(-time.Minute)
	return []availabilityCase{
		{name: "active", status: SubscribeStatusActive, expire: future, want: Available},
		{name: "legacy pending", status: SubscribeStatusPending, expire: future, want: Available},
		{name: "active no limit", status: SubscribeStatusActive, expire: NoLimitExpiry, want: Available},
		{name: "active NULL expiry", status: SubscribeStatusActive, nullEx: true, want: Available},
		{name: "active unlimited traffic", status: SubscribeStatusActive, expire: future, used: [2]int64{1 << 40, 1}, want: Available},
		{name: "active traffic left", status: SubscribeStatusActive, expire: future, used: [2]int64{50, 49}, quota: 100, want: Available},
		{name: "active expiry equals now", status: SubscribeStatusActive, expire: now, want: Expired},
		{name: "active past expiry", status: SubscribeStatusActive, expire: past, want: Expired},
		{name: "active traffic used up", status: SubscribeStatusActive, expire: future, used: [2]int64{50, 50}, quota: 100, want: TrafficExhausted},
		{name: "past expiry and used up", status: SubscribeStatusActive, expire: past, used: [2]int64{100, 0}, quota: 100, want: Expired},
		{name: "finished after an old reset", status: SubscribeStatusFinished, expire: future, want: TrafficExhausted},
		{name: "finished", status: SubscribeStatusFinished, expire: future, used: [2]int64{0, 100}, quota: 100, want: TrafficExhausted},
		{name: "expired status", status: SubscribeStatusExpired, expire: future, want: Expired},
		{name: "deducted", status: SubscribeStatusDeducted, expire: future, want: Refunded},
		{name: "stopped", status: SubscribeStatusStopped, expire: NoLimitExpiry, want: Stopped},
		{name: "stopped and expired", status: SubscribeStatusStopped, expire: past, want: Stopped},
		{name: "unknown status", status: 9, expire: future, want: Inactive},
	}
}

func (c availabilityCase) subscribe(id int64) Subscribe {
	sub := Subscribe{Id: id, Status: c.status, ExpireTime: c.expire, Traffic: c.quota, Upload: c.used[0], Download: c.used[1]}
	sub.Token, sub.UUID = fmt.Sprintf("token-%d", id), fmt.Sprintf("uuid-%d", id)
	return sub
}

func TestAvailabilityAt(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	for i, tt := range availabilityCases(now) {
		sub := tt.subscribe(int64(i + 1))
		if tt.nullEx {
			sub.ExpireTime = time.Time{}
		}
		if got := sub.AvailabilityAt(now); got != tt.want {
			t.Fatalf("%s: AvailabilityAt = %d, want %d", tt.name, got, tt.want)
		}
		if got := sub.ServableAt(now); got != (tt.want == Available) {
			t.Fatalf("%s: ServableAt = %v", tt.name, got)
		}
	}
}

// ServableCondition selects exactly the rows ServableAt accepts. The rows are
// written and read through SQLite, so NULL expiries and the epoch sentinel go
// through a real driver.
func TestServableConditionAgreesWithServableAt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE user_subscribe (
 id INTEGER PRIMARY KEY, user_id BIGINT, order_id BIGINT, subscribe_id BIGINT,
 start_time TIMESTAMP, expire_time TIMESTAMP, finished_at TIMESTAMP, traffic_reset_at TIMESTAMP,
 traffic BIGINT DEFAULT 0, download BIGINT DEFAULT 0, upload BIGINT DEFAULT 0,
 token VARCHAR(255) UNIQUE, uuid VARCHAR(255) UNIQUE, status INTEGER DEFAULT 0,
 note TEXT, entitlement_source VARCHAR(32) NOT NULL DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	cases := availabilityCases(now)
	var want []int64
	for i, tt := range cases {
		sub := tt.subscribe(int64(i + 1))
		if err := db.Create(&sub).Error; err != nil {
			t.Fatal(err)
		}
		if tt.nullEx {
			if err := db.Exec("UPDATE user_subscribe SET expire_time = NULL WHERE id = ?", sub.Id).Error; err != nil {
				t.Fatal(err)
			}
		}
		var stored Subscribe
		if err := db.First(&stored, sub.Id).Error; err != nil {
			t.Fatal(err)
		}
		if stored.ServableAt(now) {
			want = append(want, sub.Id)
		}
	}
	// Rows with NULL counters are never exhausted in either form.
	if err := db.Exec("INSERT INTO user_subscribe (id, status, expire_time, traffic, upload, download, token, uuid) VALUES (100, 1, NULL, 10, NULL, NULL, 't100', 'u100')").Error; err != nil {
		t.Fatal(err)
	}
	want = append(want, 100)

	condition, args := ServableCondition(now)
	var got []int64
	if err := db.Model(&Subscribe{}).Where(condition, args...).Order("id").Pluck("id", &got).Error; err != nil {
		t.Fatal(err)
	}
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SQL servable ids = %v, Go servable ids = %v", got, want)
	}
}

func TestResetTrafficReactivatesOnlyFinishedSubscriptionsInTerm(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	finishedAt := past
	tests := []struct {
		name       string
		sub        Subscribe
		wantStatus uint8
		wantCols   []string
	}{
		{"finished in term", Subscribe{Status: SubscribeStatusFinished, ExpireTime: future, FinishedAt: &finishedAt},
			SubscribeStatusActive, []string{"download", "upload", "status", "finished_at"}},
		{"finished without time limit", Subscribe{Status: SubscribeStatusFinished, ExpireTime: NoLimitExpiry, FinishedAt: &finishedAt},
			SubscribeStatusActive, []string{"download", "upload", "status", "finished_at"}},
		{"finished but expired", Subscribe{Status: SubscribeStatusFinished, ExpireTime: past, FinishedAt: &finishedAt},
			SubscribeStatusFinished, []string{"download", "upload"}},
		{"finished before its provider period", Subscribe{Status: SubscribeStatusFinished, StartTime: future, ExpireTime: future.Add(time.Hour)},
			SubscribeStatusFinished, []string{"download", "upload"}},
		{"active", Subscribe{Status: SubscribeStatusActive, ExpireTime: future}, SubscribeStatusActive, []string{"download", "upload"}},
		{"expired", Subscribe{Status: SubscribeStatusExpired, ExpireTime: past}, SubscribeStatusExpired, []string{"download", "upload"}},
		{"stopped", Subscribe{Status: SubscribeStatusStopped, ExpireTime: future}, SubscribeStatusStopped, []string{"download", "upload"}},
		{"deducted", Subscribe{Status: SubscribeStatusDeducted, ExpireTime: future}, SubscribeStatusDeducted, []string{"download", "upload"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := tt.sub
			sub.Upload, sub.Download = 7, 8
			columns := sub.ResetTraffic(now)
			if sub.Upload != 0 || sub.Download != 0 || sub.Status != tt.wantStatus || !reflect.DeepEqual(columns, tt.wantCols) {
				t.Fatalf("after reset: status=%d up=%d down=%d columns=%v; want status %d columns %v", sub.Status, sub.Upload, sub.Download, columns, tt.wantStatus, tt.wantCols)
			}
			if tt.wantStatus == SubscribeStatusActive && sub.FinishedAt != nil {
				t.Fatal("a reactivated subscription keeps its finished time")
			}
		})
	}
}
