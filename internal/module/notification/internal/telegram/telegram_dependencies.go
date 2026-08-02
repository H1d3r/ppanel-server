package telegram

import (
	"context"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// TelegramSessionStore reads and consumes short-lived account-binding tokens.
// Deleting the token on a successful bind is what makes the deep link
// single-use.
type TelegramSessionStore interface {
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

// TelegramRedisStore supports both account-binding sessions and administrator
// command confirmations.
type TelegramRedisStore interface {
	TelegramSessionStore
	TelegramAdminActionStore
}

// TelegramAdminHandler handles administrator Telegram commands.
type TelegramAdminHandler interface {
	Handle(msg *models.Message)
}

// TelegramLogicDependencies explicitly declares the collaborators used by
// general Telegram command dispatch and account binding.
type TelegramLogicDependencies struct {
	Messenger TelegramMessenger
	Sessions  TelegramSessionStore
	UserAuth  repository.UserAuthRepo
	UserCache repository.UserCacheRepo
	Admin     TelegramAdminHandler
}

// NewTelegramBotMessenger adapts a Telegram Bot API client to the command
// messenger port.
func NewTelegramBotMessenger(bot *tgbot.Bot) TelegramMessenger {
	return telegramBotMessenger{bot: bot}
}

// NewTelegramBotCommandRegistrar adapts a Telegram Bot API client to the
// command-menu port.
func NewTelegramBotCommandRegistrar(bot *tgbot.Bot) TelegramCommandRegistrar {
	return telegramBotMessenger{bot: bot}
}

// NewTelegramRedisStore adapts Redis to the binding-session and administrator
// confirmation ports.
func NewTelegramRedisStore(client *redis.Client) TelegramRedisStore {
	return redisTelegramStore{client: client}
}

type redisTelegramStore struct {
	client *redis.Client
}

func (s redisTelegramStore) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

func (s redisTelegramStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s redisTelegramStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}
