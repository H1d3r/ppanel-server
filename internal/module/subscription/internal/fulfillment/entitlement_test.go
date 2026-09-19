package fulfillment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/entitlement"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/module/subscription/internal/repo"
	"github.com/perfect-panel/server/internal/module/subscription/internal/selfsub"
	adminsub "github.com/perfect-panel/server/internal/module/subscription/internal/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type periodTestStore struct {
	repository.SubscriptionStore
	db         *gorm.DB
	rds        *redis.Client
	plans      *periodTestPlans
	failCommit bool
}

func (s *periodTestStore) InSubscriptionTx(ctx context.Context, fn func(repository.SubscriptionStore) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := fn(&periodTestStore{db: tx, rds: s.rds, plans: s.plans})
		if err == nil && s.failCommit {
			return errors.New("injected transaction rollback")
		}
		return err
	})
}
func (s *periodTestStore) Entitlement() repository.EntitlementRepo {
	return repo.NewEntitlementRepo(s.db)
}
func (s *periodTestStore) UserSubscription() repository.UserSubscriptionRepo {
	return repo.NewUserSubscriptionRepo(repository.ModuleConn{DB: s.db, Redis: s.rds}.Conn())
}
func (s *periodTestStore) Subscribe() repository.SubscribeRepo { return s.plans }
func (s *periodTestStore) Inbox() repository.InboxRepo         { return &periodTestInbox{db: s.db} }

type periodTestInbox struct {
	repository.InboxRepo
	db *gorm.DB
}

func (r *periodTestInbox) Find(ctx context.Context, consumer, key string) (*inbox.Record, error) {
	var row inbox.Record
	err := r.db.WithContext(ctx).First(&row, "consumer = ? AND event_key = ?", consumer, key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *periodTestInbox) Insert(ctx context.Context, consumer, key, result string) error {
	return r.db.WithContext(ctx).Create(&inbox.Record{Consumer: consumer, EventKey: key, Result: result}).Error
}

type periodTestPlans struct {
	repository.SubscribeRepo
	mu         sync.Mutex
	cleared    []int64
	traffic    int64
	resetCycle int64
}

func (p *periodTestPlans) FindOne(_ context.Context, id int64) (*subscribe.Subscribe, error) {
	traffic := p.traffic
	if traffic == 0 {
		traffic = 100
	}
	return &subscribe.Subscribe{Id: id, Name: "monthly", Traffic: traffic, UnitTime: "Month", ResetCycle: p.resetCycle}, nil
}
func (p *periodTestPlans) ClearCache(_ context.Context, ids ...int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleared = append(p.cleared, ids...)
	return nil
}

type periodTestCache struct {
	repository.UserCacheRepo
	fail bool
}

func (c *periodTestCache) ClearSubscribeCache(context.Context, ...*usersub.Subscribe) error {
	if c.fail {
		return errors.New("cache unavailable")
	}
	return nil
}

type periodTestOrders struct {
	repository.OrderRepo
	rows map[int64]*order.Order
}

func (r *periodTestOrders) FindOne(_ context.Context, id int64) (*order.Order, error) {
	row := r.rows[id]
	if row == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return row, nil
}
func (r *periodTestOrders) FindOneByOrderNo(_ context.Context, no string) (*order.Order, error) {
	for _, row := range r.rows {
		if row.OrderNo == no {
			return row, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

type periodFixture struct {
	service *Service
	store   *periodTestStore
	orders  *periodTestOrders
	cache   *periodTestCache
}

func newPeriodFixture(t *testing.T) *periodFixture {
	t.Helper()
	var dialect gorm.Dialector = sqlite.Open(filepath.Join(t.TempDir(), "periods.db"))
	if dsn := os.Getenv("ENTITLEMENT_TEST_POSTGRES_DSN"); dsn != "" {
		// The caller supplies an isolated test database, never production.
		admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		schema := "ent_" + entitlementKey(t.Name(), t.TempDir())[:20]
		if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error
			conn, _ := admin.DB()
			_ = conn.Close()
		})
		dialect = postgres.Open(dsn + " search_path=" + schema)
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	// The production user model still carries MySQL-specific default/type
	// tags. Use the equivalent portable fixture for this pre-existing table.
	idType := "INTEGER PRIMARY KEY"
	if db.Dialector.Name() == "postgres" {
		idType = "BIGSERIAL PRIMARY KEY"
	}
	if err := db.Exec(fmt.Sprintf(`CREATE TABLE user_subscribe (
 id %s, user_id BIGINT, order_id BIGINT, subscribe_id BIGINT,
 start_time TIMESTAMP, expire_time TIMESTAMP, finished_at TIMESTAMP,
 traffic BIGINT, download BIGINT DEFAULT 0, upload BIGINT DEFAULT 0,
 token VARCHAR(255) UNIQUE, uuid VARCHAR(255) UNIQUE, status INTEGER,
 note TEXT, entitlement_source VARCHAR(32) NOT NULL DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`, idType)).Error; err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&entitlement.State{}, &entitlement.Period{}, &entitlement.Revision{}, &inbox.Record{}} {
		if err := db.AutoMigrate(model); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("CREATE TABLE subscription_user_serial (user_id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	store := &periodTestStore{db: db, rds: rds, plans: &periodTestPlans{}}
	orders := &periodTestOrders{rows: map[int64]*order.Order{}}
	cache := &periodTestCache{}
	return &periodFixture{store: store, orders: orders, cache: cache, service: NewService(Deps{Store: store, Orders: orders, Plans: store.plans, UserSubs: store.UserSubscription(), Cache: cache, SingleModel: func() bool { return false }})}
}
func periodCommand() dto.ReconcileEntitlementCommand {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return dto.ReconcileEntitlementCommand{Source: "apple", Scope: "app.example:Production", SubscriptionKey: "original-1", TransactionKey: "tx-1", UserID: 7, PlanID: 1, OrderID: 1, Revision: 1, Mode: "auto_renewable", BillingInterval: "month", Status: "active", PeriodStart: now.Add(-48 * time.Hour), PeriodEnd: now.Add(28 * 24 * time.Hour), AutoRenew: true, ResetTraffic: true}
}
func (f *periodFixture) accept(c dto.ReconcileEntitlementCommand) {
	f.orders.rows[c.OrderID] = &order.Order{Id: c.OrderID, OrderNo: c.TransactionKey, TradeNo: c.TransactionKey, UserId: c.UserID, SubscribeId: c.PlanID, Method: "AppleIAP", Status: 2}
}
func (f *periodFixture) sync(t *testing.T, c dto.ReconcileEntitlementCommand) *dto.EntitlementResult {
	t.Helper()
	f.accept(c)
	r, err := f.service.ReconcileEntitlement(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f *periodFixture) sub(t *testing.T, id int64) *usersub.Subscribe {
	t.Helper()
	var sub usersub.Subscribe
	if err := f.store.db.First(&sub, id).Error; err != nil {
		t.Fatal(err)
	}
	return &sub
}
func (f *periodFixture) usage(t *testing.T, id int64, status uint8) {
	t.Helper()
	if err := f.store.db.Model(&usersub.Subscribe{}).Where("id = ?", id).Updates(map[string]any{"upload": 60, "download": 40, "status": status}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestProviderPeriodUsesAbsoluteDatesAndNeverRegrantsReplay(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	r := f.sync(t, c)
	sub := f.sub(t, r.UserSubscribeID)
	if !sub.StartTime.Equal(c.PeriodStart) || !sub.ExpireTime.Equal(c.PeriodEnd) {
		t.Fatalf("delayed delivery changed period: %+v", sub)
	}
	f.usage(t, sub.Id, usersub.SubscribeStatusFinished)
	if replay := f.sync(t, c); replay.Applied {
		t.Fatal("duplicate applied")
	}
	c.Revision++
	c.AutoRenew = false
	f.sync(t, c)
	sub = f.sub(t, sub.Id)
	if sub.Upload != 60 || sub.Download != 40 || sub.Status != usersub.SubscribeStatusFinished || !sub.ExpireTime.Equal(c.PeriodEnd) {
		t.Fatalf("cancel/replay changed quota or expiry: %+v", sub)
	}
	var count int64
	f.store.db.Model(&entitlement.Period{}).Count(&count)
	if count != 1 {
		t.Fatalf("period count %d", count)
	}
}

func TestProviderGraceRetryRevocationAndStaleSnapshots(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	c.PeriodStart = time.Now().UTC().Add(-32 * 24 * time.Hour).Truncate(time.Millisecond)
	c.PeriodEnd = c.PeriodStart.Add(30 * 24 * time.Hour)
	c.Status = "grace"
	grace := c.PeriodEnd.Add(4 * 24 * time.Hour)
	c.GraceUntil = &grace
	r := f.sync(t, c)
	if sub := f.sub(t, r.UserSubscribeID); sub.Status != 1 || !sub.ExpireTime.Equal(grace) {
		t.Fatalf("grace not projected: %+v", sub)
	}
	stale := c
	c.Revision++
	c.Status = "billing_retry"
	c.GraceUntil = nil
	f.sync(t, c)
	if sub := f.sub(t, r.UserSubscribeID); sub.Status != 3 || !sub.ExpireTime.Equal(c.PeriodEnd) {
		t.Fatalf("retry granted service: %+v", sub)
	}
	if got := f.sync(t, stale); got.Applied || got.Revision != 2 {
		t.Fatal("old grace overwrote retry")
	}
	c.Revision++
	c.Status = "active"
	c.PeriodEnd = time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond)
	f.sync(t, c)
	c.Revision++
	c.Status = "revoked"
	revoked := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	c.RevokedAt = &revoked
	f.sync(t, c)
	if sub := f.sub(t, r.UserSubscribeID); sub.Status != 3 || !sub.ExpireTime.Equal(revoked) {
		t.Fatalf("revocation did not stop access: %+v", sub)
	}
	// Only a later reconciled revision can reverse revocation.
	c.Revision++
	c.Status = "active"
	c.RevokedAt = nil
	f.sync(t, c)
	if sub := f.sub(t, r.UserSubscribeID); sub.Status != 1 {
		t.Fatalf("reversal not applied: %+v", sub)
	}
}

func TestNewPeriodResetsOnceAndPreservesAdministrativeHold(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	r := f.sync(t, c)
	f.usage(t, r.UserSubscribeID, usersub.SubscribeStatusStopped)
	c.Revision++
	c.TransactionKey = "tx-2"
	c.OrderID = 2
	c.PeriodStart = c.PeriodStart.Add(24 * time.Hour)
	c.PeriodEnd = c.PeriodEnd.Add(24 * time.Hour)
	f.sync(t, c)
	sub := f.sub(t, r.UserSubscribeID)
	if sub.Upload != 0 || sub.Download != 0 || sub.Status != usersub.SubscribeStatusStopped {
		t.Fatalf("new period or admin hold lost: %+v", sub)
	}
	f.usage(t, sub.Id, usersub.SubscribeStatusStopped)
	c.Revision++
	f.sync(t, c)
	if sub = f.sub(t, sub.Id); sub.Upload != 60 || sub.Status != usersub.SubscribeStatusStopped {
		t.Fatalf("replay reset hold: %+v", sub)
	}
}

func TestProviderOwnershipAndRevisionConflicts(t *testing.T) {
	for _, kind := range []string{"account", "transaction", "same revision", "order", "interval", "plan"} {
		t.Run(kind, func(t *testing.T) {
			f := newPeriodFixture(t)
			c := periodCommand()
			f.sync(t, c)
			switch kind {
			case "account":
				c.UserID = 8
				c.Revision++
			case "transaction":
				c.SubscriptionKey = "other-chain"
			case "same revision":
				c.AutoRenew = false
			case "order":
				c.OrderID = 2
				c.Revision++
			case "interval":
				c.BillingInterval = "year"
				c.Revision++
			case "plan":
				c.PlanID = 2
				c.Revision++
			}
			f.accept(c)
			if _, err := f.service.ReconcileEntitlement(context.Background(), c); !errors.Is(err, ErrEntitlementConflict) {
				t.Fatalf("want conflict, got %v", err)
			}
		})
	}
}

func TestProviderRollbackAndCacheRetry(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	f.accept(c)
	f.store.failCommit = true
	if _, err := f.service.ReconcileEntitlement(context.Background(), c); err == nil {
		t.Fatal("expected rollback")
	}
	for _, model := range []any{&usersub.Subscribe{}, &entitlement.State{}, &entitlement.Period{}, &entitlement.Revision{}} {
		var n int64
		f.store.db.Model(model).Count(&n)
		if n != 0 {
			t.Fatalf("partial commit %T: %d", model, n)
		}
	}
	f.store.failCommit = false
	f.cache.fail = true
	if _, err := f.service.ReconcileEntitlement(context.Background(), c); err == nil {
		t.Fatal("expected cache error")
	}
	f.cache.fail = false
	if r := f.sync(t, c); r.Applied {
		t.Fatal("cache retry reapplied grant")
	}
}

func TestAppleScopeRejectsUnsupportedProducts(t *testing.T) {
	for _, tc := range []struct{ mode, interval string }{
		{"fixed_term", "month"}, {"consumable", "month"}, {"", "month"},
		{"auto_renewable", ""}, {"auto_renewable", "week"}, {"auto_renewable", "quarter"},
	} {
		t.Run(tc.mode+"/"+tc.interval, func(t *testing.T) {
			f := newPeriodFixture(t)
			c := periodCommand()
			c.Mode = tc.mode
			c.BillingInterval = tc.interval
			f.accept(c)
			if _, err := f.service.ReconcileEntitlement(context.Background(), c); err == nil {
				t.Fatal("unsupported product accepted")
			}
			var count int64
			if err := f.store.db.Model(&entitlement.Period{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("rejected product created an entitlement")
			}
		})
	}
}

func TestMonthlyAndYearlyProductsKeepAppleDates(t *testing.T) {
	for _, interval := range []string{"month", "year"} {
		t.Run(interval, func(t *testing.T) {
			f := newPeriodFixture(t)
			c := periodCommand()
			c.BillingInterval = interval
			c.AutoRenew = false
			// Even an annual product may have a short introductory transaction.
			// The product interval must not replace the provider's actual dates.
			c.PeriodEnd = c.PeriodStart.Add(7 * 24 * time.Hour)
			r := f.sync(t, c)
			if !r.AccessUntil.Equal(c.PeriodEnd) {
				t.Fatal("provider dates replaced with nominal duration")
			}
			var period entitlement.Period
			if err := f.store.db.First(&period).Error; err != nil {
				t.Fatal(err)
			}
			if period.BillingInterval != interval {
				t.Fatal("billing interval not persisted")
			}
		})
	}
}

func TestProviderSyncRequiresVerifiedPaidOrder(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	f.accept(c)
	f.orders.rows[1].Status = 1
	if _, err := f.service.ReconcileEntitlement(context.Background(), c); !errors.Is(err, ErrEntitlementConflict) {
		t.Fatalf("unpaid order accepted: %v", err)
	}
	if _, err := f.service.FulfillPaidOrder(context.Background(), c.TransactionKey); !errors.Is(err, usersub.ErrProviderManaged) {
		t.Fatalf("Apple order entered local addition: %v", err)
	}
}

func TestLocalOrdersPersistPeriodsAndReplayWithoutExtending(t *testing.T) {
	f := newPeriodFixture(t)
	ctx := context.Background()
	f.orders.rows[1] = &order.Order{Id: 1, OrderNo: "local-1", UserId: 7, SubscribeId: 1, Status: 2, Type: 1, Quantity: 1}
	if _, err := f.service.FulfillPaidOrder(ctx, "local-1"); err != nil {
		t.Fatal(err)
	}
	var first entitlement.Period
	if err := f.store.db.First(&first).Error; err != nil {
		t.Fatal(err)
	}
	sub := f.sub(t, first.UserSubscribeID)
	f.orders.rows[2] = &order.Order{Id: 2, OrderNo: "local-2", UserId: 7, SubscribeId: 1, SubscribeToken: sub.Token, Status: 2, Type: 2, Quantity: 1}
	if _, err := f.service.FulfillPaidOrder(ctx, "local-2"); err != nil {
		t.Fatal(err)
	}
	// Operational inbox retention must not erase purchase idempotency.
	if err := f.store.db.Where("consumer = ?", inboxFulfillment).Delete(&inbox.Record{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.FulfillPaidOrder(ctx, "local-2"); err != nil {
		t.Fatal(err)
	}
	var second entitlement.Period
	if err := f.store.db.First(&second, "order_id = 2").Error; err != nil {
		t.Fatal(err)
	}
	if !second.StartAt.Equal(first.EndAt) {
		t.Fatal("renewal did not start at previous end")
	}
	if _, err := f.service.FulfillPaidOrder(ctx, "local-2"); err != nil {
		t.Fatal(err)
	}
	if got := f.sub(t, sub.Id); !got.ExpireTime.Equal(second.EndAt) {
		t.Fatal("replay extended expiry")
	}
}

func TestPostgresConcurrentEntitlementRevisions(t *testing.T) {
	if os.Getenv("ENTITLEMENT_TEST_POSTGRES_DSN") == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	f := newPeriodFixture(t)
	c := periodCommand()
	f.accept(c)
	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(rev int) {
			defer wg.Done()
			cmd := c
			cmd.Revision = int64(rev)
			cmd.AutoRenew = rev%2 == 0
			_, err := f.service.ReconcileEntitlement(context.Background(), cmd)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var state entitlement.State
	if err := f.store.db.First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Revision != n {
		t.Fatalf("revision regressed: %d", state.Revision)
	}
	for _, model := range []any{&usersub.Subscribe{}, &entitlement.State{}, &entitlement.Period{}} {
		var count int64
		f.store.db.Model(model).Count(&count)
		if count != 1 {
			t.Fatalf("duplicate %T: %d", model, count)
		}
	}
}

func TestPostgresSameTransactionCannotCreateTwoSubscriptions(t *testing.T) {
	if os.Getenv("ENTITLEMENT_TEST_POSTGRES_DSN") == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	f := newPeriodFixture(t)
	c := periodCommand()
	d := c
	d.UserID = 8
	d.SubscriptionKey = "other-chain"
	d.OrderID = 2
	f.accept(c)
	f.accept(d)
	errs := make(chan error, 2)
	start := make(chan struct{})
	for _, cmd := range []dto.ReconcileEntitlementCommand{c, d} {
		go func(cmd dto.ReconcileEntitlementCommand) {
			<-start
			_, err := f.service.ReconcileEntitlement(context.Background(), cmd)
			errs <- err
		}(cmd)
	}
	close(start)
	success := 0
	for i := 0; i < 2; i++ {
		if <-errs == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful owners = %d", success)
	}
	var count int64
	f.store.db.Model(&usersub.Subscribe{}).Count(&count)
	if count != 1 {
		t.Fatalf("orphan subscription after losing transaction: %d", count)
	}
}

func TestProviderRejectsFutureAndSubMillisecondPeriods(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	c.PeriodStart = time.Now().Add(time.Hour)
	c.PeriodEnd = c.PeriodStart.Add(time.Hour)
	f.accept(c)
	if _, err := f.service.ReconcileEntitlement(context.Background(), c); err == nil {
		t.Fatal("future period activated")
	}
	c = periodCommand()
	c.PeriodEnd = c.PeriodStart.Add(time.Nanosecond)
	f.accept(c)
	if _, err := f.service.ReconcileEntitlement(context.Background(), c); err == nil {
		t.Fatal("zero millisecond interval accepted")
	}
}

func TestProviderLocalManagementAndStaleLifecycleWorkers(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	r := f.sync(t, c)
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, &userEntity.User{Id: c.UserID})
	self := selfsub.NewService(selfsub.Deps{UserSubs: f.store.UserSubscription()})
	if err := self.Unsubscribe(ctx, &dto.UnsubscribeRequest{Id: r.UserSubscribeID}); !errors.Is(err, usersub.ErrProviderManaged) {
		t.Fatalf("wallet refund allowed: %v", err)
	}
	if _, err := self.PreUnsubscribe(ctx, &dto.PreUnsubscribeRequest{Id: r.UserSubscribeID}); !errors.Is(err, usersub.ErrProviderManaged) {
		t.Fatalf("wallet refund quote allowed: %v", err)
	}
	admin := adminsub.NewService(adminsub.Deps{UserSubs: f.store.UserSubscription()})
	if err := admin.UpdateUserSubscribe(ctx, &dto.UpdateUserSubscribeRequest{UserSubscribeId: r.UserSubscribeID}); !errors.Is(err, usersub.ErrProviderManaged) {
		t.Fatalf("admin expiry edit allowed: %v", err)
	}
	if err := admin.DeleteUserSubscribe(ctx, &dto.DeleteUserSubscribeRequest{UserSubscribeId: r.UserSubscribeID}); !errors.Is(err, usersub.ErrProviderManaged) {
		t.Fatalf("provider binding deletion allowed: %v", err)
	}
	// An expiry candidate loaded before reconciliation must not stop a
	// renewed subscription when the worker finally writes its old result.
	if err := f.store.UserSubscription().MarkSubscribesFinished(ctx, []int64{r.UserSubscribeID}, usersub.SubscribeStatusExpired, time.Now()); err != nil {
		t.Fatal(err)
	}
	if sub := f.sub(t, r.UserSubscribeID); sub.Status != 1 {
		t.Fatal("stale expiry sweep stopped renewed subscription")
	}
	reminders, err := f.store.UserSubscription().FindExpiringSubscribes(ctx, c.PeriodEnd.Add(-time.Hour), c.PeriodEnd.Add(time.Hour))
	if err != nil || len(reminders) != 0 {
		t.Fatalf("provider subscription got local-price expiry reminder: %v %+v", err, reminders)
	}
	// A reset candidate loaded before revocation/hold cannot reactivate it.
	c.Revision++
	c.Status = "revoked"
	revoked := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	c.RevokedAt = &revoked
	f.sync(t, c)
	traffic := f.store.UserSubscription().(repository.SubscriptionTrafficRepo)
	if err := traffic.ResetSubscribeTrafficByIds(ctx, []int64{r.UserSubscribeID}); err != nil {
		t.Fatal(err)
	}
	if sub := f.sub(t, r.UserSubscribeID); sub.Status != 3 {
		t.Fatal("stale reset reactivated revoked subscription")
	}
}

func TestMonthlyYearlySubscriptionsRemainIndependent(t *testing.T) {
	f := newPeriodFixture(t)
	monthly := periodCommand()
	a := f.sync(t, monthly)
	yearly := monthly
	yearly.BillingInterval = "year"
	yearly.SubscriptionKey = "yearly-chain"
	yearly.TransactionKey = "yearly-transaction"
	yearly.OrderID = 2
	yearly.PeriodEnd = yearly.PeriodStart.Add(365 * 24 * time.Hour)
	b := f.sync(t, yearly)
	if a.UserSubscribeID == b.UserSubscribeID {
		t.Fatal("independent subscriptions merged")
	}
	monthly.Revision++
	monthly.Status = "revoked"
	revoked := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	monthly.RevokedAt = &revoked
	f.sync(t, monthly)
	if sub := f.sub(t, b.UserSubscribeID); sub.Status != 1 || !sub.ExpireTime.Equal(yearly.PeriodEnd) {
		t.Fatal("monthly refund removed a separate yearly entitlement")
	}
}

func TestNewTransactionCanChangeMonthlyToYearly(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	first := f.sync(t, c)
	c.BillingInterval = "year"
	c.TransactionKey = "yearly-renewal"
	c.OrderID = 2
	c.Revision++
	c.PeriodStart = time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	c.PeriodEnd = c.PeriodStart.Add(365 * 24 * time.Hour)
	second := f.sync(t, c)
	if second.UserSubscribeID != first.UserSubscribeID || !second.AccessUntil.Equal(c.PeriodEnd) {
		t.Fatal("interval change replaced the subscription or used local duration")
	}
}

func TestExistingPeriodKeepsQuotaSnapshotAndCalendarResetIsSeparate(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	r := f.sync(t, c)
	f.store.plans.traffic = 500
	c.Revision++
	c.AutoRenew = false
	f.sync(t, c)
	if sub := f.sub(t, r.UserSubscribeID); sub.Traffic != 100 {
		t.Fatal("catalog edit changed existing period quota")
	}
	f.usage(t, r.UserSubscribeID, usersub.SubscribeStatusFinished)
	f.store.plans.resetCycle = 2
	c.Revision++
	c.TransactionKey = "tx-2"
	c.OrderID = 2
	c.PeriodStart = c.PeriodStart.Add(time.Hour)
	c.PeriodEnd = c.PeriodEnd.Add(time.Hour)
	f.sync(t, c)
	sub := f.sub(t, r.UserSubscribeID)
	if sub.Traffic != 500 || sub.Upload != 60 || sub.Download != 40 {
		t.Fatalf("calendar plan reset during billing sync: %+v", sub)
	}
}

func TestCacheRecoveryInvalidatesAllPriorPlans(t *testing.T) {
	f := newPeriodFixture(t)
	c := periodCommand()
	f.sync(t, c)
	f.store.plans.cleared = nil
	f.cache.fail = true
	for id := int64(2); id <= 3; id++ {
		c.Revision = id
		c.PlanID = id
		c.OrderID = id
		c.TransactionKey = fmt.Sprintf("tx-%d", id)
		f.accept(c)
		if _, err := f.service.ReconcileEntitlement(context.Background(), c); err == nil {
			t.Fatal("expected cache failure")
		}
	}
	f.cache.fail = false
	f.sync(t, c)
	seen := map[int64]bool{}
	for _, id := range f.store.plans.cleared {
		seen[id] = true
	}
	if len(seen) != 3 || !seen[1] || !seen[2] || !seen[3] {
		t.Fatalf("stale plan caches: %v", seen)
	}
}
