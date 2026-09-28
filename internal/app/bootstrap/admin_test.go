package bootstrap

import (
	"context"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
)

// A configured password is used as given; the documented installs configure
// none, and those must not fall back to the published default.
func TestInitialAdminPasswordGeneratesOnlyWhenUnset(t *testing.T) {
	for _, configured := range []string{"configured-secret", defaultAdminPassword} {
		if got := initialAdminPassword("admin@example.com", configured); got != configured {
			t.Fatalf("configured password %q replaced with %q", configured, got)
		}
	}
	if got := initialAdminPassword("admin@example.com", ""); got == "" || got == defaultAdminPassword || len(got) < 16 {
		t.Fatalf("initialAdminPassword(\"\") = %q, want a generated password", got)
	}
	if first, second := initialAdminPassword("admin@example.com", ""), initialAdminPassword("admin@example.com", ""); first == second {
		t.Fatal("generated passwords repeat")
	}
}

type adminUsers struct {
	repository.UserRepo
	admins []*user.User
}

func (r adminUsers) QueryAdminUsers(context.Context) ([]*user.User, error) { return r.admins, nil }

type adminStore struct {
	repository.Store
	users adminUsers
}

func (s adminStore) User() repository.UserRepo { return s.users }

func TestWarnDefaultAdminPasswordFlagsOnlyDefaultPasswords(t *testing.T) {
	logs := logtest.NewCollector(t)
	store := adminStore{users: adminUsers{admins: []*user.User{
		{Id: 1, Password: password.EncodePassWord(defaultAdminPassword), Algo: password.PasswordAlgoArgon2id,
			AuthMethods: []user.AuthMethods{{AuthType: "email", AuthIdentifier: "admin@ppanel.dev"}}},
		{Id: 2, Password: password.EncodePassWord("a-strong-password"), Algo: password.PasswordAlgoArgon2id},
	}}}

	WarnDefaultAdminPassword(&Dependencies{Store: store})

	content := logs.String()
	if !strings.Contains(content, "default password") || !strings.Contains(content, `"user_id":1`) {
		t.Fatalf("default-password administrator not flagged:\n%s", content)
	}
	if strings.Contains(content, `"user_id":2`) {
		t.Fatalf("administrator with a strong password flagged:\n%s", content)
	}
}
