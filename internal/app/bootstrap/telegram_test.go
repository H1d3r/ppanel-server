package bootstrap

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
)

type emptyTelegramTokenStore struct {
	repository.Store
	auth repository.AuthRepo
}

func (s emptyTelegramTokenStore) Auth() repository.AuthRepo { return s.auth }

type emptyTelegramTokenAuthRepo struct {
	repository.AuthRepo
}

func (emptyTelegramTokenAuthRepo) FindOneByMethod(context.Context, string) (*auth.Auth, error) {
	enabled := false
	return &auth.Auth{
		Method:  "telegram",
		Config:  `{"bot_token":"","enable_notify":false,"webhook_domain":"","group_chat_id":""}`,
		Enabled: &enabled,
	}, nil
}

type panickingNotification struct {
	notification.Service
	handled int
}

func (n *panickingNotification) HandleTelegramUpdate(context.Context, *models.Update) {
	n.handled++
	panic("handler bug")
}

// The polling loop runs handlers without recover: a panic escaping the
// update handler would kill the API process.
func TestTelegramUpdateHandlerContainsPanics(t *testing.T) {
	logs := logtest.NewCollector(t)
	notify := &panickingNotification{}
	handle := telegramUpdateHandler(&Dependencies{Notification: notify})

	handle(context.Background(), nil, &models.Update{ID: 1, Message: &models.Message{Text: "first"}})
	handle(context.Background(), nil, &models.Update{ID: 2, Message: &models.Message{Text: "second"}})
	handle(context.Background(), nil, &models.Update{ID: 3})

	if notify.handled != 2 {
		t.Fatalf("handled = %d, want both message updates dispatched and the empty one skipped", notify.handled)
	}
	if out := logs.String(); !strings.Contains(out, "update handler panicked") || !strings.Contains(out, "handler bug") || !strings.Contains(out, "telegramUpdateHandler") {
		t.Fatalf("log = %s, want the panic logged with its stack", out)
	}
}

func TestTelegramEmptyTokenClearsPublishedRuntimeState(t *testing.T) {
	runtimeConfig := config.Config{Telegram: config.Telegram{
		Enable:        true,
		BotID:         123,
		BotName:       "old-bot",
		BotToken:      "old-token",
		EnableNotify:  true,
		WebHookDomain: "https://old.example.com",
		GroupChatID:   -100123,
	}}
	botSetterCalled := false
	publishedBot := new(tgbot.Bot)
	deps := &Dependencies{
		Config: func() config.Config { return runtimeConfig },
		UpdateConfig: func(update func(*config.Config)) {
			update(&runtimeConfig)
		},
		Store: emptyTelegramTokenStore{auth: emptyTelegramTokenAuthRepo{}},
		SetTelegramBot: func(bot *tgbot.Bot) {
			botSetterCalled = true
			publishedBot = bot
		},
	}

	if err := Telegram(deps); err != nil {
		t.Fatalf("Telegram() = %v", err)
	}

	if !botSetterCalled {
		t.Fatal("Telegram() did not revoke the published bot client")
	}
	if publishedBot != nil {
		t.Fatal("Telegram() retained the previous bot client after clearing the token")
	}
	if !reflect.DeepEqual(runtimeConfig.Telegram, config.Telegram{}) {
		t.Fatalf("runtime Telegram config = %#v, want zero config", runtimeConfig.Telegram)
	}
}
