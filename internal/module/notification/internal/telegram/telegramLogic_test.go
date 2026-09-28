package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
)

// privateUpdate wraps a command typed in the private chat with chatID.
func privateUpdate(chatID int64, text string) *models.Update {
	return &models.Update{Message: telegramCommand(chatID, text)}
}

func newTrafficBot(subs *fakeSubscriptions) (*Bot, *recordingMessenger) {
	accounts := newFakeAccounts()
	accounts.addBinding(7, "telegram", "1001")
	messenger := &recordingMessenger{}
	return NewBot(BotDependencies{Messenger: messenger, Accounts: accounts, Subscriptions: subs}), messenger
}

func TestTrafficReportsActiveSubscriptions(t *testing.T) {
	subs := newFakeSubscriptions()
	expiry := time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)
	subs.byUser[7] = []*usersub.SubscribeDetails{
		{Id: 1, Status: usersub.SubscribeStatusActive, Subscribe: &subscribe.Subscribe{Name: "Pro"},
			Traffic: 10 << 30, Download: 3 << 30, Upload: 1 << 30, ExpireTime: expiry},
		{Id: 2, Status: usersub.SubscribeStatusActive, Subscribe: &subscribe.Subscribe{Name: "Lifetime"},
			ExpireTime: time.UnixMilli(0)},
		{Id: 3, Status: usersub.SubscribeStatusExpired, Subscribe: &subscribe.Subscribe{Name: "Old"}},
	}
	bot, messenger := newTrafficBot(subs)

	bot.HandleUpdate(context.Background(), privateUpdate(1001, "/traffic"))

	got := messenger.last()
	if got.chatID != 1001 {
		t.Fatalf("reply went to chat %d", got.chatID)
	}
	for _, want := range []string{"📦 Pro", "已用：4.0GB / 10.0GB", "剩余：6.0GB", "📦 Lifetime", "无限制", "长期有效"} {
		if !strings.Contains(got.message, want) {
			t.Fatalf("report = %q, want %q", got.message, want)
		}
	}
	if strings.Contains(got.message, "Old") {
		t.Fatalf("report = %q lists an expired subscription", got.message)
	}
}

func TestTrafficWithoutActiveSubscription(t *testing.T) {
	bot, messenger := newTrafficBot(newFakeSubscriptions())
	bot.HandleUpdate(context.Background(), privateUpdate(1001, "/traffic"))
	if got := messenger.last().message; got != "您当前没有生效中的订阅。" {
		t.Fatalf("reply = %q", got)
	}
}

// Traffic belongs to the bound account; an unbound chat learns nothing.
func TestTrafficRequiresBinding(t *testing.T) {
	bot, messenger := newTrafficBot(newFakeSubscriptions())
	bot.HandleUpdate(context.Background(), privateUpdate(2002, "/traffic"))
	if got := messenger.last(); got.chatID != 2002 || !strings.Contains(got.message, "请先绑定账号") {
		t.Fatalf("reply = %+v, want a bind prompt", got)
	}
}

func TestTrafficReportClampsOverusedQuota(t *testing.T) {
	report := trafficReport([]*usersub.SubscribeDetails{{
		Status: usersub.SubscribeStatusActive, Traffic: 1 << 30, Download: 2 << 30, ExpireTime: time.UnixMilli(0),
	}})
	if !strings.Contains(report, "剩余：0MB") {
		t.Fatalf("report = %q, want no negative remainder", report)
	}
}
