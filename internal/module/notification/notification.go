// Package notification is the facade of the notification module. It starts
// with the Telegram bot: update handling (webhook and polling), the unbind
// notice and the message templates other domains render. Additional channels
// (email, SMS broadcast) join as migration proceeds (ADR-001 step 4).
package notification

import (
	"context"
	"errors"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/perfect-panel/server/internal/module/notification/internal/telegram"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/redis/go-redis/v9"
)

// Service is the only surface other code may depend on; the implementation
// lives under internal/ where the compiler seals it off.
type Service interface {
	// HandleTelegramUpdate processes one bot update (command dispatch,
	// binding, admin actions). Both the webhook handler and the polling
	// loop call it.
	HandleTelegramUpdate(ctx context.Context, update *tgbotapi.Update)
	// NotifyTelegramUnbind sends the best-effort unbind notice to the chat.
	NotifyTelegramUnbind(userID, chatID int64) error
}

// Message templates other domains render before handing the text to the bot.
const (
	PurchaseNotify     = telegram.PurchaseNotify
	RenewalNotify      = telegram.RenewalNotify
	ResetTrafficNotify = telegram.ResetTrafficNotify
	RechargeNotify     = telegram.RechargeNotify
	AdminOrderNotify   = telegram.AdminOrderNotify
)

// Deps declares everything the module needs; the composition root
// (internal/svc) provides them.
type Deps struct {
	// Bot returns the current bot client; the initialize subsystem recreates
	// it when the Telegram configuration changes, so it is read per call.
	// nil means the bot is not configured.
	Bot func() *tgbotapi.BotAPI
	Redis         *redis.Client
	Users         repository.UserRepo
	UserAuth      repository.UserAuthRepo
	UserCache     repository.UserCacheRepo
	Tickets       repository.TicketRepo
	Orders        repository.OrderRepo
	Subscriptions repository.UserSubscriptionRepo
	Plans         repository.SubscribeRepo
	Logs          repository.LogRepo
}

func New(deps Deps) Service {
	return &service{deps: deps}
}

type service struct {
	deps Deps
}

func (s *service) HandleTelegramUpdate(ctx context.Context, update *tgbotapi.Update) {
	messenger := telegram.NewTelegramBotMessenger(s.deps.Bot())
	sessions := telegram.NewTelegramRedisStore(s.deps.Redis)
	admin := telegram.NewTelegramAdmin(ctx, telegram.TelegramAdminDependencies{
		Messenger:     messenger,
		Actions:       sessions,
		Tickets:       s.deps.Tickets,
		Orders:        s.deps.Orders,
		Users:         s.deps.Users,
		UserAuth:      s.deps.UserAuth,
		Subscriptions: s.deps.Subscriptions,
		UserCache:     s.deps.UserCache,
		Plans:         s.deps.Plans,
		Logs:          s.deps.Logs,
	})
	telegram.NewTelegramLogic(ctx, telegram.TelegramLogicDependencies{
		Messenger: messenger,
		Sessions:  sessions,
		UserAuth:  s.deps.UserAuth,
		UserCache: s.deps.UserCache,
		Admin:     admin,
	}).TelegramLogic(update)
}

func (s *service) NotifyTelegramUnbind(userID, chatID int64) error {
	text, err := tool.RenderTemplateToString(telegram.UnbindNotify, map[string]string{
		"Id":   strconv.FormatInt(userID, 10),
		"Time": timeutil.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return err
	}
	bot := s.deps.Bot()
	if bot == nil {
		return errors.New("telegram bot is not configured")
	}
	_, err = bot.Send(tgbotapi.NewMessage(chatID, text))
	return err
}
