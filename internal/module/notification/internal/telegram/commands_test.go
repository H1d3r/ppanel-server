package telegram

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
)

type recordedCommandMenu struct {
	chatID   int64
	commands []Command
}

type fakeCommandRegistrar struct {
	menus []recordedCommandMenu
	err   error
}

func (r *fakeCommandRegistrar) SetCommands(chatID int64, commands []Command) error {
	if r.err != nil {
		return r.err
	}
	r.menus = append(r.menus, recordedCommandMenu{chatID: chatID, commands: commands})
	return nil
}

func commandNames(commands []Command) map[string]bool {
	names := make(map[string]bool, len(commands))
	for _, command := range commands {
		names[command.Command] = true
	}
	return names
}

// The destructive administrator commands must never reach the menu every user
// sees; only binding and help belong there.
func TestPublicCommandsExcludeAdministratorEntries(t *testing.T) {
	names := commandNames(PublicCommands())
	for _, want := range []string{"start", "bind", "help"} {
		if !names[want] {
			t.Fatalf("public menu is missing /%s", want)
		}
	}
	for _, forbidden := range []string{"ban", "reset", "toggle", "dash", "user"} {
		if names[forbidden] {
			t.Fatalf("public menu exposes the administrator command /%s", forbidden)
		}
	}
}

// The administrator menu carries the full command set, and never the inline
// confirmation commands, which only make sense with a generated action id.
func TestAdminCommandsCoverTheDispatchedSet(t *testing.T) {
	names := commandNames(AdminCommands())
	dispatched := []string{
		"dash", "tickets", "tickets_waiting", "tk", "rp", "close", "reopen",
		"user", "user_sub", "user_log", "reset", "toggle", "ban", "help",
	}
	for _, want := range dispatched {
		if !names[want] {
			t.Fatalf("administrator menu is missing /%s", want)
		}
	}
	for _, forbidden := range []string{"confirm_", "cancel_"} {
		if names[forbidden] {
			t.Fatalf("administrator menu exposes the inline command /%s", forbidden)
		}
	}
}

// An authenticated administrator gets the administrator menu scoped to their
// own chat, and only once per chat so a menu update does not ride along with
// every command.
func TestAdminMenuIsPublishedOncePerChat(t *testing.T) {
	adminMenuChats.Clear()
	t.Cleanup(func() { adminMenuChats.Clear() })

	const chatID int64 = 4242
	isAdmin := true
	registrar := &fakeCommandRegistrar{}
	messenger := &fakeTelegramMessenger{}
	admin := NewTelegramAdmin(context.Background(), TelegramAdminDependencies{
		Messenger: messenger,
		Commands:  registrar,
		Users:     &fakeTelegramAdminUsers{users: map[int64]*user.User{7: {Id: 7, IsAdmin: &isAdmin}}},
		UserAuth:  &fakeTelegramAdminAuth{byChat: map[string]*user.AuthMethods{"4242": {UserId: 7}}},
	})

	admin.Handle(telegramCommand(chatID, "/help"))
	admin.Handle(telegramCommand(chatID, "/help"))

	if len(registrar.menus) != 1 {
		t.Fatalf("menu publishes = %d, want 1", len(registrar.menus))
	}
	if registrar.menus[0].chatID != chatID {
		t.Fatalf("menu scope = %d, want the administrator's chat %d", registrar.menus[0].chatID, chatID)
	}
	if !commandNames(registrar.menus[0].commands)["ban"] {
		t.Fatal("administrator menu was published without the administrator commands")
	}
}

// A chat without administrator rights must not be given the menu.
func TestAdminMenuIsNotPublishedForOrdinaryUsers(t *testing.T) {
	adminMenuChats.Clear()
	t.Cleanup(func() { adminMenuChats.Clear() })

	notAdmin := false
	registrar := &fakeCommandRegistrar{}
	admin := NewTelegramAdmin(context.Background(), TelegramAdminDependencies{
		Messenger: &fakeTelegramMessenger{},
		Commands:  registrar,
		Users:     &fakeTelegramAdminUsers{users: map[int64]*user.User{9: {Id: 9, IsAdmin: &notAdmin}}},
		UserAuth:  &fakeTelegramAdminAuth{byChat: map[string]*user.AuthMethods{"55": {UserId: 9}}},
	})

	admin.Handle(telegramCommand(55, "/dash"))

	if len(registrar.menus) != 0 {
		t.Fatalf("menu publishes = %d, want 0 for a non-administrator", len(registrar.menus))
	}
}
