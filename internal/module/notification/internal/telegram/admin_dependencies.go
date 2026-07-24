package telegram

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

// TelegramMessenger sends a response to a Telegram chat.
type TelegramMessenger interface {
	Send(chatID int64, message string) error
}

// TelegramAdminActionStore persists short-lived confirmations for destructive
// administrator commands.
type TelegramAdminActionStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// TelegramAdminDependencies contains only the collaborators used by Telegram
// administrator commands. It intentionally does not accept ServiceContext.
type TelegramAdminDependencies struct {
	Messenger     TelegramMessenger
	Actions       TelegramAdminActionStore
	Tickets       repository.TicketRepo
	Orders        repository.OrderRepo
	Users         repository.UserRepo
	UserAuth      repository.UserAuthRepo
	Subscriptions repository.UserSubscriptionRepo
	UserCache     repository.UserCacheRepo
	Plans         repository.SubscribeRepo
	Logs          repository.LogRepo
	// Wallet is the billing-domain read port for balance display.
	Wallet repository.WalletRepo
}

// TelegramAdmin handles administrative Telegram commands independently from
// the general Telegram bot flow.
type TelegramAdmin struct {
	logger.Logger
	ctx  context.Context
	deps TelegramAdminDependencies
}

func NewTelegramAdmin(ctx context.Context, deps TelegramAdminDependencies) *TelegramAdmin {
	return &TelegramAdmin{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (a *TelegramAdmin) sendMessage(message string, chatID int64) error {
	return a.deps.Messenger.Send(chatID, message)
}
