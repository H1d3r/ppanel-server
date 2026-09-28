package quotatask

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/platform/entity/task"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type quotaFailureStore struct {
	repository.Store
	subscription repository.SubscriptionStore
}

func (s *quotaFailureStore) InSubscriptionTx(_ context.Context, fn func(repository.SubscriptionStore) error) error {
	return fn(s.subscription)
}

type quotaSubscriptionStore struct {
	repository.SubscriptionStore
	users repository.UserSubscriptionRepo
	inbox repository.InboxRepo
}

func (s *quotaSubscriptionStore) UserSubscription() repository.UserSubscriptionRepo { return s.users }
func (s *quotaSubscriptionStore) Inbox() repository.InboxRepo                       { return s.inbox }
func (s *quotaSubscriptionStore) Log() repository.LogRepo                           { return quotaLogRepo{} }

type quotaLogRepo struct{ repository.LogRepo }

func (quotaLogRepo) Insert(context.Context, *logEntity.SystemLog) error { return nil }

// quotaSubscriptionRepo holds one stored row: the grant re-reads it under
// lock and writes back the named columns.
type quotaSubscriptionRepo struct {
	repository.UserSubscriptionRepo
	row     *usersub.Subscribe
	err     error
	columns []string
}

func newQuotaSubscriptionRepo(row usersub.Subscribe) *quotaSubscriptionRepo {
	return &quotaSubscriptionRepo{row: &row}
}

func (r *quotaSubscriptionRepo) FindOneSubscribeForUpdate(_ context.Context, id int64) (*usersub.Subscribe, error) {
	if r.row == nil || r.row.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	row := *r.row
	return &row, nil
}

func (r *quotaSubscriptionRepo) UpdateSubscribeColumns(_ context.Context, data *usersub.Subscribe, columns ...string) error {
	if r.err != nil {
		return r.err
	}
	r.columns = append(r.columns, columns...)
	row := *data
	r.row = &row
	return nil
}

type quotaInbox struct {
	repository.InboxRepo
	inserts int
}

type quotaGiftReplayStore struct {
	repository.Store
	inbox        repository.InboxRepo
	cache        repository.UserCacheRepo
	billingCalls int
}

func (s *quotaGiftReplayStore) Inbox() repository.InboxRepo         { return s.inbox }
func (s *quotaGiftReplayStore) UserCache() repository.UserCacheRepo { return s.cache }
func (s *quotaGiftReplayStore) InBillingTx(_ context.Context, _ func(repository.BillingStore) error) error {
	s.billingCalls++
	return errors.New("billing transaction should not run for a completed gift stage")
}

type existingQuotaInbox struct{ repository.InboxRepo }

func (existingQuotaInbox) Find(context.Context, string, string) (*inbox.Record, error) {
	return &inbox.Record{Consumer: inboxQuotaGift, EventKey: "7:9"}, nil
}

type recordingUserCache struct {
	repository.UserCacheRepo
	cleared int
}

func (c *recordingUserCache) ClearUserCache(_ context.Context, users ...*userEntity.User) error {
	c.cleared += len(users)
	return nil
}

func (r *quotaInbox) Find(context.Context, string, string) (*inbox.Record, error) { return nil, nil }
func (r *quotaInbox) Insert(context.Context, string, string, string) error {
	r.inserts++
	return nil
}

func TestGrantSubscriptionDoesNotMarkInboxAfterUpdateFailure(t *testing.T) {
	wantErr := errors.New("write failed")
	sub := &usersub.Subscribe{Id: 9, ExpireTime: time.Now().Add(time.Hour)}
	users := newQuotaSubscriptionRepo(*sub)
	users.err = wantErr
	marks := &quotaInbox{}
	store := &quotaFailureStore{subscription: &quotaSubscriptionStore{users: users, inbox: marks}}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	err := logic.grantSubscription(context.Background(), 7, sub, task.QuotaContent{Days: 1}, time.Now())
	if err == nil || marks.inserts != 0 {
		t.Fatalf("update error must roll back without an inbox marker: err=%v inserts=%d", err, marks.inserts)
	}
}

func TestGrantSubscriptionReactivatesTrafficFinishedSubscription(t *testing.T) {
	finishedAt := time.Now()
	sub := &usersub.Subscribe{
		Id: 9, Status: usersub.SubscribeStatusFinished, FinishedAt: &finishedAt,
		Download: 10, Upload: 20,
	}
	users := newQuotaSubscriptionRepo(*sub)
	marks := &quotaInbox{}
	store := &quotaFailureStore{subscription: &quotaSubscriptionStore{users: users, inbox: marks}}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	if err := logic.grantSubscription(context.Background(), 7, sub, task.QuotaContent{ResetTraffic: true}, time.Now()); err != nil {
		t.Fatalf("grantSubscription: %v", err)
	}
	stored := users.row
	if stored.Status != usersub.SubscribeStatusActive || stored.FinishedAt != nil || stored.Download != 0 || stored.Upload != 0 || marks.inserts != 1 {
		t.Fatalf("reset quota did not reactivate subscription atomically: row=%+v inserts=%d", stored, marks.inserts)
	}
	if fmt.Sprint(users.columns) != "[download upload status finished_at]" {
		t.Fatalf("reset quota wrote columns %v", users.columns)
	}
	if *sub != *stored {
		t.Fatalf("caller copy was not refreshed: sub=%+v row=%+v", sub, stored)
	}
}

func TestGrantSubscriptionPreservesNoLimitExpiry(t *testing.T) {
	sub := &usersub.Subscribe{Id: 9, Status: usersub.SubscribeStatusActive, ExpireTime: time.UnixMilli(0)}
	users := newQuotaSubscriptionRepo(*sub)
	marks := &quotaInbox{}
	store := &quotaFailureStore{subscription: &quotaSubscriptionStore{users: users, inbox: marks}}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	if err := logic.grantSubscription(context.Background(), 7, sub, task.QuotaContent{Days: 30}, time.Now()); err != nil {
		t.Fatalf("grantSubscription: %v", err)
	}
	if stored := users.row; stored.ExpireTime.UnixMilli() != 0 || stored.Status != usersub.SubscribeStatusActive || len(users.columns) != 0 || marks.inserts != 1 {
		t.Fatalf("NoLimit subscription was downgraded: row=%+v columns=%v inserts=%d", stored, users.columns, marks.inserts)
	}
}

func TestGrantGiftReplaySkipsPlanLookupAndRefreshesCache(t *testing.T) {
	cache := &recordingUserCache{}
	store := &quotaGiftReplayStore{inbox: existingQuotaInbox{}, cache: cache}
	logic := &QuotaTaskLogic{deps: Deps{Store: store}}

	err := logic.grantGift(context.Background(), 7, &usersub.Subscribe{Id: 9, UserId: 11, SubscribeId: 13}, task.QuotaContent{GiftType: 2, GiftValue: 10}, time.Now())
	if err != nil {
		t.Fatalf("completed gift replay: %v", err)
	}
	if store.billingCalls != 0 || cache.cleared != 1 {
		t.Fatalf("completed gift stage replayed work: billing_calls=%d cache_clears=%d", store.billingCalls, cache.cleared)
	}
}

func TestValidateContentRejectsInvalidQuotaActions(t *testing.T) {
	for _, content := range []task.QuotaContent{
		{},
		{GiftType: 1},
		{GiftType: 9, GiftValue: 1},
		{GiftType: 1, GiftValue: ^uint64(0)},
	} {
		if err := validateContent(content); err == nil {
			t.Fatalf("invalid content accepted: %+v", content)
		}
	}
	if err := validateContent(task.QuotaContent{Days: 1}); err != nil {
		t.Fatalf("valid quota action rejected: %v", err)
	}
}
