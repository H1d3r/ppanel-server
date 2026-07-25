package repository

import (
	"context"

	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// The store is assembled from per-module repository bundles (ADR-001 step-6
// preparation: each module owns its persistence implementation and exports a
// builder; this package keeps only the contracts and the assembly).
//
// A builder runs once for the root connection and once per transaction with
// the tx-scoped connection. Anything that must survive across transactions
// (cache-retry singletons and similar) belongs in the closure that produced
// the builder, not in the bundle.

// ModuleConn is the per-connection context handed to a repo builder.
type ModuleConn struct {
	DB    *gorm.DB
	Redis *redis.Client
	// Invalidations batches cache invalidation keys during a transaction;
	// nil outside transactions.
	Invalidations *cache.InvalidationQueue
}

// Conn builds the cached connection every repository implementation wraps.
func (c ModuleConn) Conn() cache.CachedConn {
	return newCachedConn(c.DB, c.Redis, c.Invalidations)
}

// SubscriptionCacheBridge is the identity bundle's window onto the
// subscription domain's cache concerns: the user-deletion cascade collects
// the user's subscription rows, and the UserCache facade delegates the
// subscription cache operations. The subscription bundle provides it.
type SubscriptionCacheBridge interface {
	QueryUserSubscribe(ctx context.Context, userId int64, status ...int64) ([]*usersub.SubscribeDetails, error)
	ClearSubscribeCache(ctx context.Context, data ...*usersub.Subscribe) error
	UpdateUserSubscribeCache(ctx context.Context, data *usersub.Subscribe) error
}

// PlatformRepos is the shared-kernel bundle.
type PlatformRepos struct {
	System SystemRepo
	Logs   LogRepo
	Tasks  TaskRepo
	Client ClientRepo
	Inbox  InboxRepo
	Outbox OutboxRepo
}

type PlatformBuilder func(conn ModuleConn) PlatformRepos

// BillingRepos is the billing domain bundle.
type BillingRepos struct {
	Orders      OrderRepo
	OrderEvents OrderEventRepo
	Payments    PaymentRepo
	Coupons     CouponRepo
	Withdrawals UserWithdrawalRepo
	Wallets     WalletRepo
}

type BillingBuilder func(conn ModuleConn) BillingRepos

// SubscriptionRepos is the subscription domain bundle.
type SubscriptionRepos struct {
	Plans    SubscribeRepo
	UserSubs UserSubscriptionRepo
	Traffic  SubscriptionTrafficRepo
	// CacheBridge feeds the identity bundle's cross-domain cache cascade.
	CacheBridge SubscriptionCacheBridge
}

type SubscriptionBuilder func(conn ModuleConn) SubscriptionRepos

// IdentityRepos is the identity domain bundle. UserCache is the cross-domain
// cache facade (its subscription keys come through the injected reader).
type IdentityRepos struct {
	Users     UserRepo
	UserAuths UserAuthRepo
	Devices   UserDeviceRepo
	UserCache UserCacheRepo
	Auths     AuthRepo
}

type IdentityBuilder func(conn ModuleConn, subs SubscriptionCacheBridge) IdentityRepos

// NetworkRepos is the network domain bundle.
type NetworkRepos struct {
	Nodes   NodeRepo
	Traffic TrafficRepo
}

type NetworkBuilder func(conn ModuleConn) NetworkRepos

// SupportRepos is the support domain bundle.
type SupportRepos struct {
	Tickets       TicketRepo
	Announcements AnnouncementRepo
	Ads           AdsRepo
	Documents     DocumentRepo
}

type SupportBuilder func(conn ModuleConn) SupportRepos

// Builders carries every module's repo builder for store assembly.
type Builders struct {
	Platform     PlatformBuilder
	Billing      BillingBuilder
	Subscription SubscriptionBuilder
	Identity     IdentityBuilder
	Network      NetworkBuilder
	Support      SupportBuilder
}
