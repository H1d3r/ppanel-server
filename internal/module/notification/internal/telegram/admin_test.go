package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type sentTelegramMessage struct {
	chatID  int64
	message string
}

type fakeTelegramMessenger struct {
	messages []sentTelegramMessage
}

func (m *fakeTelegramMessenger) Send(chatID int64, message string) error {
	m.messages = append(m.messages, sentTelegramMessage{chatID: chatID, message: message})
	return nil
}

type fakeTelegramActions struct {
	values  map[string]string
	deleted []string
}

func (s *fakeTelegramActions) Get(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", gorm.ErrRecordNotFound
	}
	return value, nil
}

func (s *fakeTelegramActions) Set(_ context.Context, key, value string, _ time.Duration) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *fakeTelegramActions) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	delete(s.values, key)
	return nil
}

type fakeTelegramAdminUsers struct {
	repository.UserRepo
	users   map[int64]*user.User
	updated *user.User
}

func (r *fakeTelegramAdminUsers) FindOne(_ context.Context, id int64) (*user.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *u
	return &copy, nil
}

func (r *fakeTelegramAdminUsers) Update(_ context.Context, data *user.User, _ ...*gorm.DB) error {
	copy := *data
	r.updated = &copy
	r.users[data.Id] = &copy
	return nil
}

type fakeTelegramAdminAuth struct {
	repository.UserAuthRepo
	byChat map[string]*user.AuthMethods
}

func (r *fakeTelegramAdminAuth) FindUserAuthMethodByOpenID(_ context.Context, _, openID string) (*user.AuthMethods, error) {
	auth, ok := r.byChat[openID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *auth
	return &copy, nil
}

func telegramCommand(chatID int64, command string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		Text: command,
		Entities: []tgbotapi.MessageEntity{{
			Type:   "bot_command",
			Offset: 0,
			Length: len(command),
		}},
	}
}

func TestTelegramAdminRejectsUnboundChat(t *testing.T) {
	messenger := &fakeTelegramMessenger{}
	admin := NewTelegramAdmin(context.Background(), TelegramAdminDependencies{
		Messenger: messenger,
		UserAuth:  &fakeTelegramAdminAuth{byChat: map[string]*user.AuthMethods{}},
	})

	admin.Handle(telegramCommand(42, "/help"))

	if len(messenger.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messenger.messages))
	}
	if !strings.Contains(messenger.messages[0].message, "尚未绑定") {
		t.Fatalf("rejection = %q, want unbound notice", messenger.messages[0].message)
	}
}

func TestTelegramAdminConfirmBanUsesOnlyInjectedPorts(t *testing.T) {
	const (
		chatID   = int64(42)
		adminID  = int64(1)
		targetID = int64(9)
		actionID = "ban-action"
	)
	adminFlag := true
	targetEnabled := true
	action, err := json.Marshal(tgAction{Cmd: "ban", AdminID: adminID, Target: "9"})
	if err != nil {
		t.Fatalf("marshal action: %v", err)
	}
	messenger := &fakeTelegramMessenger{}
	actions := &fakeTelegramActions{values: map[string]string{tgActionPrefix + actionID: string(action)}}
	users := &fakeTelegramAdminUsers{users: map[int64]*user.User{
		adminID:  {Id: adminID, IsAdmin: &adminFlag},
		targetID: {Id: targetID, Enable: &targetEnabled},
	}}
	admin := NewTelegramAdmin(context.Background(), TelegramAdminDependencies{
		Messenger: messenger,
		Actions:   actions,
		Users:     users,
		UserAuth:  &fakeTelegramAdminAuth{byChat: map[string]*user.AuthMethods{"42": {UserId: adminID}}},
	})

	admin.Handle(telegramCommand(chatID, "/confirm_"+actionID))

	if users.updated == nil || users.updated.Enable == nil || *users.updated.Enable {
		t.Fatalf("updated user = %#v, want disabled target", users.updated)
	}
	if len(actions.deleted) != 1 || actions.deleted[0] != tgActionPrefix+actionID {
		t.Fatalf("deleted actions = %#v, want confirmation token removed", actions.deleted)
	}
	if len(messenger.messages) != 1 || !strings.Contains(messenger.messages[0].message, "已禁用") {
		t.Fatalf("messages = %#v, want disabled confirmation", messenger.messages)
	}
}
