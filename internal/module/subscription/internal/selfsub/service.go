// Package selfsub implements the user self-service subscription management
// of the subscription module: viewing, token reset, notes, and the two-phase
// cancellation with its billing refund. Only the module facade may reach it.
package selfsub

import (
	"context"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
)

type Deps struct {
	UserSubs repository.UserSubscriptionRepo
	Plans    repository.SubscribeRepo
	// Users, Orders, Logs and Inbox are read ports onto the identity,
	// billing and platform domains.
	Users  BuyerReader
	Orders OrderReader
	Cache  CacheInvalidator
	Logs   LogReader
	Inbox  MarkerReader
	Store  Store
	// SingleModel forbids holding more than one blocking subscription;
	// runtime-mutable, read per request.
	SingleModel func() bool
}

// BuyerReader reads the account whose referrer a refund charges back.
type BuyerReader interface {
	FindOne(ctx context.Context, id int64) (*user.User, error)
}

// OrderReader reads an order with its renewals: what a refund pays back.
type OrderReader interface {
	FindOneDetails(ctx context.Context, id int64) (*order.Details, error)
}

// CacheInvalidator drops cached subscription rows.
type CacheInvalidator interface {
	ClearSubscribeCache(ctx context.Context, data ...*usersub.Subscribe) error
}

// LogReader pages the platform's system log.
type LogReader interface {
	FilterSystemLog(ctx context.Context, filter *log.FilterParams) ([]*log.SystemLog, int64, error)
}

// MarkerReader reads the inbox markers of the two cancellation stages.
type MarkerReader interface {
	Find(ctx context.Context, consumer, eventKey string) (*inbox.Record, error)
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) GetSubscribeLog(ctx context.Context, req *dto.GetSubscribeLogRequest) (*dto.GetSubscribeLogResponse, error) {
	return newGetSubscribeLogLogic(ctx, s.deps).GetSubscribeLog(req)
}

func (s *Service) PreUnsubscribe(ctx context.Context, req *dto.PreUnsubscribeRequest) (*dto.PreUnsubscribeResponse, error) {
	return newPreUnsubscribeLogic(ctx, s.deps).PreUnsubscribe(req)
}

// Store is the persistence capability required by this package: the
// cancellation's subscription-domain transaction and the refund's
// billing-domain transaction. NewStore adapts the application store.
type Store interface {
	repository.SubscriptionTransactor
	// InRefundTx runs fn in a billing-domain transaction over the refund's
	// ledger.
	InRefundTx(ctx context.Context, fn func(RefundLedger) error) error
}

// RefundLedger is what a refund settlement reads and writes inside its
// billing-domain transaction.
type RefundLedger interface {
	// LockWallet reads the user's wallet under a row lock.
	LockWallet(ctx context.Context, userID int64) (*wallet.Wallet, error)
	// SaveBalance persists the wallet's balance and gift amount.
	SaveBalance(ctx context.Context, w *wallet.Wallet) error
	// SaveCommission persists the wallet's commission.
	SaveCommission(ctx context.Context, w *wallet.Wallet) error
	// OrderDetails reads the order with its renewals.
	OrderDetails(ctx context.Context, orderID int64) (*order.Details, error)
	// AppendLog records a balance, gift or commission movement.
	AppendLog(ctx context.Context, entry *log.SystemLog) error
	// MarkRefunded records that the subscription's refund was settled; a
	// second marker for the same subscription fails.
	MarkRefunded(ctx context.Context, subKey string) error
}

// AppStore is the part of the application store NewStore adapts.
type AppStore interface {
	repository.SubscriptionTransactor
	repository.BillingTransactor
}

// NewStore adapts the application store to this package's Store.
func NewStore(store AppStore) Store {
	return appStore{store}
}

type appStore struct {
	AppStore
}

func (s appStore) InRefundTx(ctx context.Context, fn func(RefundLedger) error) error {
	return s.InBillingTx(ctx, func(store repository.BillingStore) error {
		return fn(billingLedger{store: store})
	})
}

// billingLedger is the refund ledger over a billing-domain transaction.
type billingLedger struct {
	store repository.BillingStore
}

func (b billingLedger) LockWallet(ctx context.Context, userID int64) (*wallet.Wallet, error) {
	return b.store.Wallet().FindOneForUpdate(ctx, userID)
}

func (b billingLedger) SaveBalance(ctx context.Context, w *wallet.Wallet) error {
	return b.store.Wallet().UpdateBalanceFields(ctx, w)
}

func (b billingLedger) SaveCommission(ctx context.Context, w *wallet.Wallet) error {
	return b.store.Wallet().UpdateCommission(ctx, w)
}

func (b billingLedger) OrderDetails(ctx context.Context, orderID int64) (*order.Details, error) {
	return b.store.Order().FindOneDetails(ctx, orderID)
}

func (b billingLedger) AppendLog(ctx context.Context, entry *log.SystemLog) error {
	return b.store.Log().Insert(ctx, entry)
}

func (b billingLedger) MarkRefunded(ctx context.Context, subKey string) error {
	return b.store.Inbox().Insert(ctx, unsubscribeRefundConsumer, subKey, "")
}
