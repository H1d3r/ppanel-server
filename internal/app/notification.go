package app

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription"
	subscriptiondto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/module/support"
	supportdto "github.com/perfect-panel/server/internal/module/support/contract"
	"github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

// newNotificationModule wires the notification module; the bot client is
// runtime-recreated, so the module reads it per call. The bot's ports are
// backed by the owning domains' repositories, except ticket changes, which
// run through the support facade so they are mirrored like any other.
func newNotificationModule(store repository.Store, srv *Application) notification.Service {
	return notification.New(notification.Deps{
		Bot:         srv.Runtime.TelegramBot,
		GroupChatID: func() int64 { return srv.Runtime.Config().Telegram.GroupChatID },
		Topics:      store.TelegramTopic(),
		Redis:       srv.Redis,
		Accounts: botAccounts{
			users: store.User(),
			auth:  store.UserAuth(),
			cache: store.UserCache(),
		},
		Tickets: botTickets{repo: store.Ticket(), support: srv.Support},
		Subscriptions: botSubscriptions{
			subs:         store.UserSubscription(),
			subscription: srv.Subscription,
		},
		Billing:   botBilling{orders: store.Order(), wallets: store.Wallet()},
		AuditLogs: botAuditLogs{logs: store.Log()},
	})
}

// botAccounts backs the bot's identity port with the identity repositories.
type botAccounts struct {
	users repository.UserRepo
	auth  repository.UserAuthRepo
	cache repository.UserCacheRepo
}

func (a botAccounts) FindUser(ctx context.Context, id int64) (*user.User, error) {
	return a.users.FindOne(ctx, id)
}

func (a botAccounts) FindBinding(ctx context.Context, authType, identifier string) (*user.AuthMethods, error) {
	return a.auth.FindUserAuthMethodByOpenID(ctx, authType, identifier)
}

func (a botAccounts) FindUserBinding(ctx context.Context, userID int64, authType string) (*user.AuthMethods, error) {
	return a.auth.FindUserAuthMethodByUserId(ctx, authType, userID)
}

func (a botAccounts) ListBindings(ctx context.Context, userID int64) ([]*user.AuthMethods, error) {
	return a.auth.FindUserAuthMethods(ctx, userID)
}

func (a botAccounts) BindTelegram(ctx context.Context, userID int64, chatID string) error {
	now := timeutil.Now()
	if err := a.auth.InsertUserAuthMethods(ctx, &user.AuthMethods{
		UserId:         userID,
		AuthType:       "telegram",
		AuthIdentifier: chatID,
		Verified:       true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return err
	}
	// The binding is stored; a stale cache entry only delays its visibility.
	if err := a.cache.ClearUserCache(ctx, &user.User{Id: userID}); err != nil {
		logger.WithContext(ctx).Errorw("[Telegram] refresh user cache after bind failed",
			logger.Field("error", err.Error()), logger.Field("user_id", userID))
	}
	return nil
}

func (a botAccounts) SetEnabled(ctx context.Context, userID int64, enabled bool) error {
	return a.users.UpdateColumns(ctx, userID, map[string]any{"enable": enabled})
}

func (a botAccounts) CountRegistrations(ctx context.Context, t time.Time) (int64, error) {
	return a.users.QueryRegisterUserTotalByDate(ctx, t)
}

// botTickets reads tickets from the support repository and applies ticket
// changes through the support facade, whose notifier mirrors them into the
// ticket's topic.
type botTickets struct {
	repo    repository.TicketRepo
	support support.Service
}

func (t botTickets) CountAwaitingReply(ctx context.Context) (int64, error) {
	return t.repo.QueryWaitReplyTotal(ctx)
}

func (t botTickets) List(ctx context.Context, page, size int, status *uint8) (int64, []*ticket.Ticket, error) {
	return t.repo.QueryTicketList(ctx, page, size, 0, status, "")
}

func (t botTickets) Find(ctx context.Context, id int64) (*ticket.Ticket, error) {
	return t.repo.FindOne(ctx, id)
}

func (t botTickets) Detail(ctx context.Context, id int64) (*ticket.Details, error) {
	return t.repo.QueryTicketDetail(ctx, id)
}

func (t botTickets) Reply(ctx context.Context, id int64, from, content string, inTopic bool) (uint8, error) {
	result, err := t.support.UpdateTicketAsStaff(ctx, &supportdto.StaffTicketUpdateCommand{
		TicketId:   id,
		Reply:      content,
		From:       from,
		FromMirror: inTopic,
	})
	if err != nil {
		return 0, err
	}
	return result.PreviousStatus, nil
}

func (t botTickets) SetStatus(ctx context.Context, id int64, status uint8, inTopic bool) error {
	_, err := t.support.UpdateTicketAsStaff(ctx, &supportdto.StaffTicketUpdateCommand{
		TicketId:   id,
		Status:     status,
		FromMirror: inTopic,
	})
	return err
}

// botSubscriptions backs the bot's subscription port. Writes go through the
// subscription facade, the same use cases the admin panel's endpoints run
// (row lock, column update, cache invalidation), so the bot cannot drift
// from them; reads use the subscription repository.
type botSubscriptions struct {
	subs         repository.UserSubscriptionRepo
	subscription subscription.Service
}

func (s botSubscriptions) Find(ctx context.Context, id int64) (*usersub.Subscribe, error) {
	return s.subs.FindOneSubscribe(ctx, id)
}

func (s botSubscriptions) ListByUser(ctx context.Context, userID int64) ([]*usersub.SubscribeDetails, error) {
	return s.subs.QueryUserSubscribe(ctx, userID)
}

func (s botSubscriptions) ResetTraffic(ctx context.Context, sub *usersub.Subscribe) error {
	return s.subscription.ResetUserSubscribeTraffic(ctx, &subscriptiondto.ResetUserSubscribeTrafficRequest{UserSubscribeId: sub.Id})
}

// SetStatus moves the subscription from the status the operator confirmed
// to status; it fails if the subscription changed in between.
func (s botSubscriptions) SetStatus(ctx context.Context, sub *usersub.Subscribe, status uint8) error {
	return s.subscription.ChangeUserSubscribeStatus(ctx, sub.Id, sub.Status, status)
}

// botBilling backs the bot's billing port with the billing repositories.
type botBilling struct {
	orders  repository.OrderRepo
	wallets repository.WalletRepo
}

func (b botBilling) Revenue(ctx context.Context, t time.Time) (int64, error) {
	total, err := b.orders.QueryDateOrders(ctx, t)
	return total.AmountTotal, err
}

func (b botBilling) Balance(ctx context.Context, userID int64) (int64, error) {
	wallet, err := b.wallets.FindWallet(ctx, userID)
	if err != nil || wallet == nil {
		return 0, err
	}
	return wallet.Balance, nil
}

// botAuditLogs backs the bot's audit-log port with the platform log
// repository.
type botAuditLogs struct {
	logs repository.LogRepo
}

func (l botAuditLogs) RecentLogins(ctx context.Context, userID int64, limit int) ([]*log.SystemLog, error) {
	entries, _, err := l.logs.FilterSystemLog(ctx, &log.FilterParams{
		Page:     1,
		Size:     limit,
		Type:     log.TypeLogin.Uint8(),
		ObjectID: userID,
	})
	return entries, err
}
