package profile

import (
	"context"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/module/identity/internal/identitytest"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthflow"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/xerr"
)

// fixedProvider vouches for one identity.
type fixedProvider struct{ subject string }

func (p *fixedProvider) AuthURL(_, state string) (string, error) {
	return "https://provider.example/authorize?state=" + state, nil
}

func (p *fixedProvider) Identify(context.Context, oauthprovider.Callback) (*oauthprovider.Identity, error) {
	return &oauthprovider.Identity{Subject: p.subject}, nil
}

type bindFixture struct {
	*identitytest.Env
	svc      *Service
	provider *fixedProvider
}

func newBindFixture(t *testing.T) *bindFixture {
	t.Helper()
	logtest.Discard(t)
	env := identitytest.New(t)
	env.EnableMethod(t, "telegram", "{}")
	provider := &fixedProvider{subject: "42"}
	svc := NewService(Deps{
		Users:     env.Store.User(),
		UserAuth:  env.Store.UserAuth(),
		Auth:      env.Store.Auth(),
		UserCache: env.Store.UserCache(),
		Redis:     env.Redis,
		Store:     env.Store,
		Policy:    registerpolicy.New(registerpolicy.Deps{Auths: env.Store.Auth(), Config: func() registerpolicy.Snapshot { return registerpolicy.Snapshot{} }}),
		OAuth: oauthflow.New(oauthflow.Deps{
			Auths: env.Store.Auth(),
			Redis: env.Redis,
			Providers: oauthprovider.Registry{
				"telegram": {New: func(string) (oauthprovider.Provider, error) { return provider, nil }},
			},
		}),
	})
	return &bindFixture{Env: env, svc: svc, provider: provider}
}

func (f *bindFixture) user(t *testing.T) *user.User {
	t.Helper()
	enabled := true
	u := &user.User{Enable: &enabled}
	if err := f.DB.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func (f *bindFixture) bind(u *user.User) error {
	return f.svc.BindOAuthCallback(user.NewContext(context.Background(), u), &dto.BindOAuthCallbackRequest{
		Method: "telegram", Callback: map[string]any{"tgAuthResult": "signed"},
	})
}

func assertCode(t *testing.T, err error, want uint32) {
	t.Helper()
	if got := xerr.CodeOf(err); err == nil || got != want {
		t.Fatalf("error = %v (code %d), want code %d", err, got, want)
	}
}

// Binding stores the identity verified; binding it again succeeds, as a
// client retrying a timed-out callback does.
func TestBindOAuthCallbackBindsTheIdentityOnce(t *testing.T) {
	f := newBindFixture(t)
	owner := f.user(t)

	if err := f.bind(owner); err != nil {
		t.Fatalf("BindOAuthCallback() error = %v", err)
	}
	if err := f.bind(owner); err != nil {
		t.Fatalf("binding the same identity again: %v", err)
	}
	identities := f.Identities(t, owner.Id)
	if len(identities) != 1 || identities[0].AuthType != "telegram" || identities[0].AuthIdentifier != "42" || !identities[0].Verified {
		t.Fatalf("identities = %+v", identities)
	}
}

// An identity belongs to one account, and an account holds one identity of a
// method.
func TestBindOAuthCallbackRefusesTakenIdentitiesAndSecondOnes(t *testing.T) {
	f := newBindFixture(t)
	owner, other := f.user(t), f.user(t)
	if err := f.bind(owner); err != nil {
		t.Fatal(err)
	}

	assertCode(t, f.bind(other), xerr.UserExist)
	f.provider.subject = "43"
	assertCode(t, f.bind(owner), xerr.UserExist)
	if n := len(f.Identities(t, owner.Id)); n != 1 {
		t.Fatalf("owner identities = %d, want 1", n)
	}
	if n := len(f.Identities(t, other.Id)); n != 0 {
		t.Fatalf("other identities = %d, want none", n)
	}
}

func TestBindOAuthCallbackNeedsASignedInUserAndAnObject(t *testing.T) {
	f := newBindFixture(t)
	err := f.svc.BindOAuthCallback(context.Background(), &dto.BindOAuthCallbackRequest{Method: "telegram", Callback: map[string]any{}})
	assertCode(t, err, xerr.InvalidAccess)
	err = f.svc.BindOAuthCallback(user.NewContext(context.Background(), f.user(t)), &dto.BindOAuthCallbackRequest{Method: "telegram", Callback: "not-an-object"})
	assertCode(t, err, xerr.InvalidParams)
	err = f.svc.BindOAuthCallback(user.NewContext(context.Background(), f.user(t)), &dto.BindOAuthCallbackRequest{Method: "google", Callback: map[string]any{}})
	assertCode(t, err, xerr.GetAuthenticatorError)
}

func TestBindOAuthStartsTheProviderRoundTrip(t *testing.T) {
	f := newBindFixture(t)
	resp, err := f.svc.BindOAuth(context.Background(), &dto.BindOAuthRequest{Method: "telegram", Redirect: "https://panel.example/bind"})
	if err != nil || resp.Redirect == "" {
		t.Fatalf("BindOAuth() = %+v, %v", resp, err)
	}
}
