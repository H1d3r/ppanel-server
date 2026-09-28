package telegram

import (
	"context"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/notification/entity/telegramtopic"
	"github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/internal/repository"
)

// The web client and the support module write user follows as "User"; only
// other authors are staff. Older rows spell it in lowercase or leave it empty.
var followAuthors = map[string]bool{"User": true, "user": true, "": true, "System": false, "admin": false}

func TestTicketRepliedLabelsAuthorsCorrectly(t *testing.T) {
	for from, isUser := range followAuthors {
		repo := &fakeTopicRepo{}
		if err := repo.Insert(context.Background(), &telegramtopic.Topic{
			ChatId: testGroupID, Kind: telegramtopic.KindTicket, RefId: 321, ThreadId: 12, Status: telegramtopic.StatusActive,
		}); err != nil {
			t.Fatal(err)
		}
		messenger := &sinkMessenger{}
		svc := NewTopicService(context.Background(), &fakeTopicClient{}, repo, testGroupID)

		if err := svc.TicketReplied(messenger, 321, from, "hi"); err != nil {
			t.Fatalf("from %q: TicketReplied: %v", from, err)
		}
		want := "💻 网站回复（管理员）"
		if isUser {
			want = "👤 用户回复"
		}
		if len(messenger.sent) != 1 || !strings.HasPrefix(messenger.sent[0].message, want) {
			t.Fatalf("from %q: sent = %+v, want the %q label", from, messenger.sent, want)
		}
	}
}

type fakeDetailTickets struct {
	repository.TicketRepo
	details *ticket.Details
}

func (f *fakeDetailTickets) QueryTicketDetail(context.Context, int64) (*ticket.Details, error) {
	return f.details, nil
}

type fakeAuthWithoutMethods struct {
	*fakeTelegramAdminAuth
}

func (fakeAuthWithoutMethods) FindUserAuthMethods(context.Context, int64) ([]*user.AuthMethods, error) {
	return nil, nil
}

func TestTicketDetailLabelsAuthorsCorrectly(t *testing.T) {
	for from, isUser := range followAuthors {
		yes := true
		messenger := &fakeTelegramMessenger{}
		admin := NewTelegramAdmin(context.Background(), TelegramAdminDependencies{
			Messenger: messenger,
			Tickets: &fakeDetailTickets{details: &ticket.Details{
				Id: 321, Title: "help", UserId: 7, Status: ticket.Pending,
				Follows: []ticket.Follow{{From: from, Content: "hi"}},
			}},
			Users:    &fakeTelegramAdminUsers{users: map[int64]*user.User{1: {Id: 1, IsAdmin: &yes, Enable: &yes}}},
			UserAuth: fakeAuthWithoutMethods{&fakeTelegramAdminAuth{byChat: map[string]*user.AuthMethods{"42": {UserId: 1}}}},
		})

		msg := telegramCommand(42, "/tk")
		msg.Text = "/tk 321"
		admin.Handle(msg)

		want := "📝 客服"
		if isUser {
			want = "📝 用户"
		}
		if len(messenger.messages) != 1 || !strings.Contains(messenger.messages[0].message, want) {
			t.Fatalf("from %q: messages = %+v, want the %q label", from, messenger.messages, want)
		}
	}
}
