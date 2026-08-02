package initialize

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/perfect-panel/server/pkg/logger"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/telegramsecret"
)

// telegramPoll tracks the long-polling loop across re-initialisations. The
// previous poller must stop before a new bot starts, otherwise two getUpdates
// consumers race on the same token and Telegram rejects one with 409.
var telegramPoll struct {
	sync.Mutex
	cancel context.CancelFunc
}

// swapTelegramPoller cancels the previous polling loop and, when start is
// non-nil, installs the next one. Cancel and install share one critical
// section so two concurrent re-initialisations cannot leak a poller.
func swapTelegramPoller(start func(ctx context.Context)) {
	telegramPoll.Lock()
	defer telegramPoll.Unlock()
	if telegramPoll.cancel != nil {
		telegramPoll.cancel()
		telegramPoll.cancel = nil
	}
	if start != nil {
		ctx, cancel := context.WithCancel(context.Background())
		telegramPoll.cancel = cancel
		go start(ctx)
	}
}

// Telegram (re)initialises the bot from the stored configuration. On failure
// it returns without touching the running state, so a transient error during
// re-initialisation leaves the previous bot working instead of none at all.
func Telegram(svc *svc.ServiceContext) {
	method, err := svc.Store.Auth().FindOneByMethod(context.Background(), "telegram")
	if err != nil {
		logger.Errorf("[Init Telegram Config] Get Telegram Config Error: %s", err.Error())
		return
	}
	tgConfig := new(auth.TelegramAuthConfig)
	if err = tgConfig.Unmarshal(method.Config); err != nil {
		logger.Errorf("[Init Telegram Config] Unmarshal Telegram Config Error: %s", err.Error())
		return
	}

	if tgConfig.BotToken == "" {
		// The bot is deliberately unconfigured: stop a leftover poller.
		swapTelegramPoller(nil)
		logger.Debug("[Init Telegram Config] Telegram Token is empty")
		return
	}

	bot, err := tgbot.New(tgConfig.BotToken,
		// GetMe runs explicitly below so its result can feed the config.
		tgbot.WithSkipGetMe(),
		// The previous library processed updates strictly in order; handlers
		// rely on that for double-submit protection (bind tokens, admin
		// confirmations), so keep them synchronous.
		tgbot.WithNotAsyncHandlers(),
		// Only message updates are handled. Narrowing the subscription also
		// keeps unknown update payloads (which the library refuses to
		// decode) from ever reaching the polling loop.
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{models.AllowedUpdateMessage}),
		tgbot.WithErrorsHandler(func(err error) {
			logger.Error("[Telegram Bot] update transport error", logger.Field("error", err.Error()))
		}),
		tgbot.WithDefaultHandler(func(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
			if update.Message != nil {
				// Detach from the poller's lifetime: cancelling the poller on
				// re-initialisation must not abort an update mid-handling.
				svc.Notification.HandleTelegramUpdate(context.WithoutCancel(ctx), update)
			}
		}),
	)
	if err != nil {
		logger.Error("[Init Telegram Config] New Bot API Error: ", logger.Field("error", err.Error()))
		return
	}

	// This runs synchronously inside startup and the admin settings request,
	// so it gets a deadline instead of the HTTP client's 60s default.
	getMeCtx, cancelGetMe := context.WithTimeout(context.Background(), 5*time.Second)
	user, err := bot.GetMe(getMeCtx)
	cancelGetMe()
	if err != nil {
		logger.Error("[Init Telegram Config] Get Bot Info Error: ", logger.Field("error", err.Error()))
		return
	}

	// The group id is parsed leniently: a malformed value logs and behaves
	// like "no group configured" instead of failing the whole bot.
	var groupChatID int64
	if raw := strings.TrimSpace(tgConfig.GroupChatID); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			logger.Errorf("[Init Telegram Config] Group chat id %q is not a number", raw)
		} else {
			groupChatID = id
		}
	}

	newConfig := config.Telegram{
		Enable:        method.Enabled != nil && *method.Enabled,
		BotID:         user.ID,
		BotName:       user.Username,
		BotToken:      tgConfig.BotToken,
		EnableNotify:  tgConfig.EnableNotify,
		WebHookDomain: tgConfig.WebHookDomain,
		GroupChatID:   groupChatID,
	}

	// Pick mode: prefer webhook, fall back to long-polling when no domain or in debug.
	useWebhook := tgConfig.WebHookDomain != "" && !svc.Config.Debug
	if useWebhook {
		// Webhook mode: register the URL with Telegram. The secret travels in
		// the X-Telegram-Bot-Api-Secret-Token header, never in the URL. The
		// config is published only after Telegram accepted the registration,
		// so a failure here cannot leave the handler expecting a secret
		// Telegram does not send yet.
		webhookURL := fmt.Sprintf("%s/v1/telegram/webhook", tgConfig.WebHookDomain)
		if _, err = bot.SetWebhook(context.Background(), &tgbot.SetWebhookParams{
			URL:            webhookURL,
			SecretToken:    telegramsecret.Derive(tgConfig.BotToken),
			AllowedUpdates: []string{models.AllowedUpdateMessage},
		}); err != nil {
			logger.Errorf("[Init Telegram Config] Request Webhook Error: %s", err.Error())
			return
		}
		swapTelegramPoller(nil)
		svc.Config.Telegram = newConfig
		svc.TelegramBot = bot
		logger.Info("[Init Telegram Config] Webhook registered", logger.Field("url", webhookURL))
	} else {
		// Long-polling mode. A leftover webhook registration blocks
		// getUpdates, so drop it first.
		if _, err = bot.DeleteWebhook(context.Background(), &tgbot.DeleteWebhookParams{}); err != nil {
			logger.Errorf("[Init Telegram Config] Delete Webhook Error: %s", err.Error())
		}
		// Publish the client and config before any update can arrive: the
		// update handlers reach the bot through svc.TelegramBot.
		svc.Config.Telegram = newConfig
		svc.TelegramBot = bot
		swapTelegramPoller(func(ctx context.Context) { bot.Start(ctx) })
		mode := "long-polling"
		if svc.Config.Debug {
			mode = "long-polling (debug)"
		}
		logger.Info("[Init Telegram Config] Using " + mode)
	}

	// Publish the command menu so the composer offers the bot's commands.
	// It is set on the bot itself, so it must be re-published whenever the
	// bot is (re)initialised. A failure is not fatal: the commands work
	// without a menu.
	if err := svc.Notification.PublishTelegramCommands(); err != nil {
		logger.Error("[Init Telegram Config] Publish Commands Error: ", logger.Field("error", err.Error()))
	}

	// The administrators' group hosts the notification topic, the support
	// and ticket topics, and the admin commands. When it is unusable, group
	// features switch off rather than degrade: the group is their only
	// channel by design.
	if groupChatID != 0 {
		if err := svc.Notification.SetupTelegramGroup(context.Background()); err != nil {
			logger.Error("[Init Telegram Config] Admin group unusable, group features disabled",
				logger.Field("group_chat_id", groupChatID),
				logger.Field("error", err.Error()))
			svc.Config.Telegram.GroupChatID = 0
		} else {
			logger.Info("[Init Telegram Config] Admin group ready", logger.Field("group_chat_id", groupChatID))
		}
	}

	logger.Info("[Init Telegram Config] Telegram init success")
}
