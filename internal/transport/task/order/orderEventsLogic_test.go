package order

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/module/platform/entity/outbox"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPublishOrderEventsDeliversDurableOutboxThenMarksPublished(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:publish-order-events?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&order.Event{}, &inbox.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("migrate event: %v", err)
	}
	event := &order.Event{OrderID: 1, OrderNo: "outbox-order", EventType: "order.created", Payload: `{}`}
	if err := db.Create(event).Error; err != nil {
		t.Fatalf("seed event: %v", err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	pubsub := redisClient.Subscribe(context.Background(), order.EventChannel(event.OrderNo))
	t.Cleanup(func() { _ = pubsub.Close() })
	if _, err := pubsub.Receive(context.Background()); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	logic := NewPublishOrderEventsLogic(Dependencies{Store: newOrderEventStore(db, redisClient), Redis: redisClient})
	if err := logic.ProcessTask(context.Background(), asynq.NewTask("test", nil)); err != nil {
		t.Fatalf("publish outbox: %v", err)
	}
	select {
	case message := <-pubsub.Channel():
		if message.Payload != strconv.FormatInt(event.ID, 10) {
			t.Fatalf("published payload = %q, want event id %d", message.Payload, event.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive published order event")
	}
	var latest order.Event
	if err := db.First(&latest, event.ID).Error; err != nil {
		t.Fatalf("reload event: %v", err)
	}
	if latest.PublishedAt == nil {
		t.Fatal("published event did not receive published_at")
	}
}

// Design: when publishing fails the outbox scan publishes the event later,
// and a repeated publication never changes the order.
func TestPublishOrderEventsRepublishesAfterAFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:republish-order-events?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&order.Order{}, &order.Event{}, &inbox.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = redisClient.Close() })
	store := newOrderEventStore(db, redisClient)
	paid := &order.Order{OrderNo: "republish", Status: order.StatusPending, StateVersion: 1}
	if err := db.Create(paid).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Order().MarkOrderPaid(context.Background(), paid.OrderNo, "trade-1"); err != nil {
		t.Fatal(err)
	}
	logic := NewPublishOrderEventsLogic(Dependencies{Store: store, Redis: redisClient})
	unpublished := func() int {
		events, err := store.OrderEvent().ListUnpublished(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(events)
	}

	redisServer.Close()
	if err := logic.ProcessTask(context.Background(), asynq.NewTask("test", nil)); err == nil {
		t.Fatal("publishing without Redis succeeded")
	}
	if unpublished() != 1 {
		t.Fatal("the failed publication marked the event published")
	}
	if err := redisServer.Restart(); err != nil {
		t.Fatal(err)
	}
	pubsub := redisClient.Subscribe(context.Background(), order.EventChannel(paid.OrderNo))
	t.Cleanup(func() { _ = pubsub.Close() })
	if _, err := pubsub.Receive(context.Background()); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	for range 2 {
		// A crash between publishing and marking publishes the event again.
		if err := db.Model(&order.Event{}).Where("order_no = ?", paid.OrderNo).Update("published_at", nil).Error; err != nil {
			t.Fatal(err)
		}
		if err := logic.ProcessTask(context.Background(), asynq.NewTask("test", nil)); err != nil {
			t.Fatalf("publish outbox: %v", err)
		}
		select {
		case <-pubsub.Channel():
		case <-time.After(time.Second):
			t.Fatal("the event was not published")
		}
	}
	if unpublished() != 0 {
		t.Fatal("the published event stayed in the outbox")
	}
	var latest order.Order
	if err := db.Where("order_no = ?", paid.OrderNo).First(&latest).Error; err != nil {
		t.Fatal(err)
	}
	if latest.Status != order.StatusPaid || latest.StateVersion != 2 {
		t.Fatalf("order = status %d version %d, want the paid state unchanged", latest.Status, latest.StateVersion)
	}
}

func TestCleanupOrderEventsKeepsUnpublishedRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cleanup-order-events?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&order.Event{}, &inbox.Record{}, &outbox.Event{}); err != nil {
		t.Fatalf("migrate event: %v", err)
	}
	old := time.Now().Add(-orderEventRetention - time.Hour)
	publishedAt := old
	published := &order.Event{OrderID: 1, OrderNo: "old-published", EventType: "order.created", Payload: `{}`, CreatedAt: old, PublishedAt: &publishedAt}
	unpublished := &order.Event{OrderID: 2, OrderNo: "old-unpublished", EventType: "order.created", Payload: `{}`, CreatedAt: old}
	if err := db.Create(published).Error; err != nil {
		t.Fatalf("seed published event: %v", err)
	}
	if err := db.Create(unpublished).Error; err != nil {
		t.Fatalf("seed unpublished event: %v", err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	logic := NewCleanupOrderEventsLogic(Dependencies{Store: newOrderEventStore(db, redisClient)})
	if err := logic.ProcessTask(context.Background(), asynq.NewTask("test", nil)); err != nil {
		t.Fatalf("cleanup events: %v", err)
	}
	var events []order.Event
	if err := db.Find(&events).Error; err != nil {
		t.Fatalf("reload events: %v", err)
	}
	if len(events) != 1 || events[0].OrderNo != unpublished.OrderNo {
		t.Fatalf("remaining events = %#v, want only unpublished event", events)
	}
}

func newOrderEventStore(db *gorm.DB, redisClient *redis.Client) *repository.GormStore {
	return repository.NewGormStoreWithBuilders(db, redisClient, repository.Builders{
		Platform:     platform.NewRepoBuilder(),
		Billing:      billing.NewRepoBuilder(),
		Subscription: subscription.NewRepoBuilder(),
		Identity:     identity.NewRepoBuilder(),
		Network:      network.NewRepoBuilder(redisClient),
		Support:      support.NewRepoBuilder(),
		Notification: notification.NewRepoBuilder(),
	})
}
