// Package notification is the facade of the notification module. It starts
// with the Telegram bot: update handling (webhook and polling), the unbind
// notice and the message templates other domains render. Additional channels
// (email, SMS broadcast) join as migration proceeds (ADR-001 step 4).
package notification

import (
	"context"
	"errors"
	"fmt"
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
	// NotifyTelegramUser sends already-rendered text to the user's bound
	// Telegram chat. It reports an error when the user has no binding or the
	// bot is unconfigured, which callers treat as "nothing to deliver".
	NotifyTelegramUser(ctx context.Context, userID int64, text string) error
	// PublishTelegramCommands registers the command menu every user sees.
	// The bot initialiser calls it once the client is ready, so the composer
	// offers the commands instead of leaving users to guess them.
	PublishTelegramCommands() error
}

// Message templates other domains render before handing the text to the bot.
const (
	PurchaseNotify        = telegram.PurchaseNotify
	RenewalNotify         = telegram.RenewalNotify
	ResetTrafficNotify    = telegram.ResetTrafficNotify
	RechargeNotify        = telegram.RechargeNotify
	AdminOrderNotify      = telegram.AdminOrderNotify
	AdminOrderDaily       = telegram.AdminOrderDaily
	SubscribeExpireNotify = telegram.SubscribeExpireNotify
)

// Deps declares everything the module needs; the composition root
// (internal/svc) provides them.
type Deps struct {
	// Bot returns the current bot client; the initialize subsystem recreates
	// it when the Telegram configuration changes, so it is read per call.
	// nil means the bot is not configured.
	Bot           func() *tgbotapi.BotAPI
	Redis         *redis.Client
	Users         repository.UserRepo
	UserAuth      repository.UserAuthRepo
	UserCache     repository.UserCacheRepo
	Tickets       repository.TicketRepo
	Orders        repository.OrderRepo
	Subscriptions repository.UserSubscriptionRepo
	Plans         repository.SubscribeRepo
	Logs          repository.LogRepo
	// Wallet is the billing-domain read port for balance display.
	Wallet repository.WalletRepo
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
		Commands:      telegram.NewTelegramBotCommandRegistrar(s.deps.Bot()),
		Actions:       sessions,
		Tickets:       s.deps.Tickets,
		Orders:        s.deps.Orders,
		Users:         s.deps.Users,
		UserAuth:      s.deps.UserAuth,
		Subscriptions: s.deps.Subscriptions,
		UserCache:     s.deps.UserCache,
		Plans:         s.deps.Plans,
		Logs:          s.deps.Logs,
		Wallet:        s.deps.Wallet,
	})
	telegram.NewTelegramLogic(ctx, telegram.TelegramLogicDependencies{
		Messenger: messenger,
		Sessions:  sessions,
		UserAuth:  s.deps.UserAuth,
		UserCache: s.deps.UserCache,
		Admin:     admin,
	}).TelegramLogic(update)
}

func (s *service) PublishTelegramCommands() error {
	bot := s.deps.Bot()
	if bot == nil {
		return errors.New("telegram bot is not configured")
	}
	return telegram.NewTelegramBotCommandRegistrar(bot).
		SetCommands(0, telegram.PublicCommands())
}

func (s *service) NotifyTelegramUser(ctx context.Context, userID int64, text string) error {
	bot := s.deps.Bot()
	if bot == nil {
		return errors.New("telegram bot is not configured")
	}
	method, err := s.deps.UserAuth.FindUserAuthMethodByUserId(ctx, "telegram", userID)
	if err != nil {
		return err
	}
	chatID, err := strconv.ParseInt(method.AuthIdentifier, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram chat id %q is malformed: %w", method.AuthIdentifier, err)
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "markdown"
	_, err = bot.Send(msg)
	return err
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
