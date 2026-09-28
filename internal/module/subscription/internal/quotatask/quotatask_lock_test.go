package quotatask

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/platform/entity/task"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/module/subscription/internal/repo"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/perfect-panel/server/pkg/timeutil"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// sqlQuotaStore runs the subscription stage against a real user_subscribe
// table, so the tests observe what a grant writes to the stored row.
type sqlQuotaStore struct {
	repository.Store
	db           *gorm.DB
	inbox        *quotaInbox
	billingCalls int
}

func (s *sqlQuotaStore) InSubscriptionTx(ctx context.Context, fn func(repository.SubscriptionStore) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&sqlQuotaSubscriptionStore{db: tx, inbox: s.inbox})
	})
}

func (s *sqlQuotaStore) InBillingTx(context.Context, func(repository.BillingStore) error) error {
	s.billingCalls++
	return errors.New("unexpected billing transaction")
}

func (s *sqlQuotaStore) InPlatformTx(_ context.Context, fn func(repository.PlatformStore) error) error {
	return fn(quotaPlatformStore{})
}

func (s *sqlQuotaStore) Inbox() repository.InboxRepo { return s.inbox }

type sqlQuotaSubscriptionStore struct {
	repository.SubscriptionStore
	db    *gorm.DB
	inbox repository.InboxRepo
}

func (s *sqlQuotaSubscriptionStore) UserSubscription() repository.UserSubscriptionRepo {
	return repo.NewUserSubscriptionRepo(repository.ModuleConn{DB: s.db, Invalidations: cache.NewInvalidationQueue()}.Conn())
}
func (s *sqlQuotaSubscriptionStore) Inbox() repository.InboxRepo { return s.inbox }
func (s *sqlQuotaSubscriptionStore) Log() repository.LogRepo     { return quotaLogRepo{} }

type quotaPlatformStore struct{ repository.PlatformStore }

func (quotaPlatformStore) Task() repository.TaskRepo { return quotaTaskRepo{} }

type quotaTaskRepo struct{ repository.TaskRepo }

func (quotaTaskRepo) UpdateActiveProgress(context.Context, *task.Task) (bool, error) {
	return true, nil
}

func newSQLQuotaStore(t *testing.T, row usersub.Subscribe) *sqlQuotaStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
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
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return &sqlQuotaStore{db: db, inbox: &quotaInbox{}}
}

func (s *sqlQuotaStore) stored(t *testing.T, id int64) usersub.Subscribe {
	t.Helper()
	var row usersub.Subscribe
	if err := s.db.First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

// The task reads its targets once when it starts. A grant applied later must
// not write that snapshot back over traffic usage, credentials or an admin
// hold that changed in between.
func TestGrantSubscriptionKeepsConcurrentWritesAndAdminHold(t *testing.T) {
	now := timeutil.Now().Truncate(time.Millisecond)
	term := now.Add(24 * time.Hour)
	store := newSQLQuotaStore(t, usersub.Subscribe{
		Id: 9, UserId: 3, SubscribeId: 1, Status: usersub.SubscribeStatusStopped, ExpireTime: term,
		Traffic: 1000, Upload: 500, Download: 700, Token: "token-fresh", UUID: "uuid-fresh",
	})
	stale := &usersub.Subscribe{
		Id: 9, UserId: 3, SubscribeId: 1, Status: usersub.SubscribeStatusActive, ExpireTime: term,
		Traffic: 1000, Token: "token-stale", UUID: "uuid-stale",
	}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	if err := logic.grantSubscription(context.Background(), 7, stale, task.QuotaContent{Days: 30}, now); err != nil {
		t.Fatalf("grantSubscription: %v", err)
	}
	row := store.stored(t, 9)
	if row.Status != usersub.SubscribeStatusStopped {
		t.Fatalf("grant lifted the admin hold: status=%d", row.Status)
	}
	if row.Upload != 500 || row.Download != 700 || row.Token != "token-fresh" || row.UUID != "uuid-fresh" {
		t.Fatalf("grant overwrote concurrent writes with its snapshot: %+v", row)
	}
	if !row.ExpireTime.Equal(term.AddDate(0, 0, 30)) {
		t.Fatalf("expire_time = %v, want %v", row.ExpireTime, term.AddDate(0, 0, 30))
	}
	if stale.Token != "token-fresh" || stale.Status != usersub.SubscribeStatusStopped || store.inbox.inserts != 1 {
		t.Fatalf("caller copy not refreshed or marker missing: sub=%+v inserts=%d", stale, store.inbox.inserts)
	}
}

func TestGrantSubscriptionResetKeepsAdminHold(t *testing.T) {
	now := timeutil.Now().Truncate(time.Millisecond)
	store := newSQLQuotaStore(t, usersub.Subscribe{
		Id: 9, UserId: 3, Status: usersub.SubscribeStatusStopped, ExpireTime: now.Add(time.Hour),
		Traffic: 1000, Upload: 500, Download: 700, Token: "token", UUID: "uuid",
	})
	stale := &usersub.Subscribe{Id: 9, UserId: 3, Status: usersub.SubscribeStatusFinished}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	if err := logic.grantSubscription(context.Background(), 7, stale, task.QuotaContent{ResetTraffic: true}, now); err != nil {
		t.Fatalf("grantSubscription: %v", err)
	}
	row := store.stored(t, 9)
	if row.Status != usersub.SubscribeStatusStopped || row.Upload != 0 || row.Download != 0 {
		t.Fatalf("reset must clear usage and keep the hold: %+v", row)
	}
}

func TestGrantSubscriptionSkipsRowDeductedAfterTaskStart(t *testing.T) {
	now := timeutil.Now().Truncate(time.Millisecond)
	term := now.Add(time.Hour)
	store := newSQLQuotaStore(t, usersub.Subscribe{
		Id: 9, UserId: 3, Status: usersub.SubscribeStatusDeducted, ExpireTime: term, Token: "token", UUID: "uuid",
	})
	stale := &usersub.Subscribe{Id: 9, UserId: 3, Status: usersub.SubscribeStatusActive, ExpireTime: term}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	err := logic.grantSubscription(context.Background(), 7, stale, task.QuotaContent{Days: 30}, now)
	if !errors.Is(err, errQuotaIneligible) {
		t.Fatalf("grantSubscription error = %v, want errQuotaIneligible", err)
	}
	row := store.stored(t, 9)
	if row.Status != usersub.SubscribeStatusDeducted || !row.ExpireTime.Equal(term) || store.inbox.inserts != 0 {
		t.Fatalf("deducted row was granted: %+v inserts=%d", row, store.inbox.inserts)
	}
}

func TestGrantSubscriptionReactivatesExpiredSubscription(t *testing.T) {
	now := timeutil.Now().Truncate(time.Millisecond)
	finishedAt := now.Add(-time.Hour)
	store := newSQLQuotaStore(t, usersub.Subscribe{
		Id: 9, UserId: 3, Status: usersub.SubscribeStatusExpired, ExpireTime: now.Add(-2 * time.Hour),
		FinishedAt: &finishedAt, Token: "token", UUID: "uuid",
	})
	stale := &usersub.Subscribe{Id: 9, UserId: 3, Status: usersub.SubscribeStatusExpired}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	if err := logic.grantSubscription(context.Background(), 7, stale, task.QuotaContent{Days: 10}, now); err != nil {
		t.Fatalf("grantSubscription: %v", err)
	}
	row := store.stored(t, 9)
	if row.Status != usersub.SubscribeStatusActive || row.FinishedAt != nil || !row.ExpireTime.Equal(now.AddDate(0, 0, 10)) {
		t.Fatalf("expired subscription was not extended from now and reactivated: %+v", row)
	}
}

// A subscription deducted after the task read its targets is reported like
// one deducted up front: no time grant and no gift.
func TestProcessSubscribesReportsRowDeductedMidTask(t *testing.T) {
	now := timeutil.Now().Truncate(time.Millisecond)
	store := newSQLQuotaStore(t, usersub.Subscribe{
		Id: 9, UserId: 3, Status: usersub.SubscribeStatusDeducted, ExpireTime: now.Add(time.Hour), Token: "token", UUID: "uuid",
	})
	stale := &usersub.Subscribe{Id: 9, UserId: 3, Status: usersub.SubscribeStatusActive, ExpireTime: now.Add(time.Hour)}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}
	taskInfo := &task.Task{Id: 7, Status: task.StatusInProgress, Total: 1}

	content := task.QuotaContent{Days: 1, GiftType: 1, GiftValue: 100}
	if err := logic.processSubscribes(context.Background(), []*usersub.Subscribe{stale}, content, taskInfo); err != nil {
		t.Fatalf("processSubscribes: %v", err)
	}
	if store.billingCalls != 0 {
		t.Fatalf("gift granted to a deducted subscription: billing calls=%d", store.billingCalls)
	}
	if taskInfo.Current != 1 || !strings.Contains(taskInfo.Errors, "deducted") {
		t.Fatalf("deducted subscription not reported: current=%d errors=%q", taskInfo.Current, taskInfo.Errors)
	}
	if row := store.stored(t, 9); row.Status != usersub.SubscribeStatusDeducted {
		t.Fatalf("deducted row changed: %+v", row)
	}
}
