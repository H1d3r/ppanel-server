package oauthflow

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// storedConfigs is the auth_method table: method → stored configuration.
type storedConfigs map[string]string

func (c storedConfigs) FindOneByMethod(_ context.Context, method string) (*auth.Auth, error) {
	config, ok := c[method]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &auth.Auth{Method: method, Config: config}, nil
}

// recorder is a provider that records the round trip.
type recorder struct {
	redirect, state string
	callback        oauthprovider.Callback
	identity        oauthprovider.Identity
	err             error
	deadline        bool
}

func (r *recorder) AuthURL(redirect, state string) (string, error) {
	r.redirect, r.state = redirect, state
	return "https://provider.example/authorize?state=" + url.QueryEscape(state), nil
}

func (r *recorder) Identify(ctx context.Context, callback oauthprovider.Callback) (*oauthprovider.Identity, error) {
	r.callback = callback
	_, r.deadline = ctx.Deadline()
	if r.err != nil {
		return nil, r.err
	}
	identity := r.identity
	return &identity, nil
}

func newFlow(t *testing.T, siteHost string, methods oauthprovider.Registry, configs storedConfigs) (*Flow, *miniredis.Miniredis) {
	t.Helper()
	logtest.Discard(t)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(Deps{Auths: configs, Redis: client, Providers: methods, SiteHost: func() string { return siteHost }}), server
}

func stateBased(provider *recorder) oauthprovider.Registry {
	return oauthprovider.Registry{"github": {State: true, New: func(string) (oauthprovider.Provider, error) { return provider, nil }}}
}

func assertCode(t *testing.T, err error, want uint32) {
	t.Helper()
	if got := xerr.CodeOf(err); err == nil || got != want {
		t.Fatalf("error = %v (code %d), want code %d", err, got, want)
	}
}

// The state issued with the URL carries the redirect to the callback, which
// redeems it once.
func TestStateTiesTheCallbackToItsURL(t *testing.T) {
	provider := &recorder{identity: oauthprovider.Identity{Subject: "583231"}}
	flow, _ := newFlow(t, "", stateBased(provider), storedConfigs{"github": "{}"})
	ctx := context.Background()

	if _, err := flow.AuthURL(ctx, "github", "https://panel.example/oauth"); err != nil {
		t.Fatalf("AuthURL() error = %v", err)
	}
	if provider.state == "" || provider.redirect != "https://panel.example/oauth" {
		t.Fatalf("provider saw state %q and redirect %q", provider.state, provider.redirect)
	}
	callback := map[string]any{"code": "authorization-code", "state": provider.state}
	identity, err := flow.Identify(ctx, "github", callback)
	if err != nil || identity.Subject != "583231" {
		t.Fatalf("Identify() = %+v, %v", identity, err)
	}
	if provider.callback.Code != "authorization-code" || provider.callback.Redirect != "https://panel.example/oauth" || !provider.deadline {
		t.Fatalf("provider callback = %+v (deadline %v)", provider.callback, provider.deadline)
	}
	_, err = flow.Identify(ctx, "github", callback)
	assertCode(t, err, xerr.OAuthStateInvalid)
	_, err = flow.Identify(ctx, "github", map[string]any{"code": "authorization-code"})
	assertCode(t, err, xerr.InvalidParams)
}

func TestProviderFailuresReachTheClientAsTheirCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want uint32
	}{
		{fmt.Errorf("%w: no", oauthprovider.ErrRejected), xerr.OAuthProviderError},
		{fmt.Errorf("%w: bad hash", oauthprovider.ErrInvalidCallback), xerr.OAuthCallbackInvalid},
		{fmt.Errorf("%w: old", oauthprovider.ErrExpiredCallback), xerr.OAuthCallbackExpired},
		{fmt.Errorf("%w: key", oauthprovider.ErrMisconfigured), xerr.OAuthProviderMisconfigured},
		{fmt.Errorf("%w: field", oauthprovider.ErrIncompleteCallback), xerr.InvalidParams},
		{errors.New("anything else"), xerr.OAuthProviderError},
	} {
		provider := &recorder{err: tc.err}
		flow, _ := newFlow(t, "", stateBased(provider), storedConfigs{"github": "{}"})
		if _, err := flow.AuthURL(context.Background(), "github", "https://panel.example/oauth"); err != nil {
			t.Fatal(err)
		}
		_, err := flow.Identify(context.Background(), "github", map[string]any{"code": "c", "state": provider.state})
		assertCode(t, err, tc.want)
		if !errors.Is(err, tc.err) {
			t.Fatalf("the provider's error is not reachable from %v", err)
		}
	}
}

func TestUnknownAndUnconfiguredMethods(t *testing.T) {
	flow, _ := newFlow(t, "", stateBased(&recorder{}), storedConfigs{})
	_, err := flow.AuthURL(context.Background(), "linkedin", "https://panel.example/oauth")
	assertCode(t, err, xerr.AuthenticatorNotSupportedError)
	_, err = flow.Identify(context.Background(), "linkedin", map[string]any{})
	assertCode(t, err, xerr.AuthenticatorNotSupportedError)
	_, err = flow.AuthURL(context.Background(), "github", "https://panel.example/oauth")
	assertCode(t, err, xerr.DatabaseQueryError)
}

// A callback without a state (Telegram) is redeemed once; a retry within the
// grace window still passes, a later replay does not.
func TestSingleUseCallbacksAreRedeemedOnce(t *testing.T) {
	provider := &recorder{identity: oauthprovider.Identity{Subject: "42", ReplayKey: "auth:telegram_callback:fingerprint"}}
	methods := oauthprovider.Registry{"telegram": {New: func(string) (oauthprovider.Provider, error) { return provider, nil }}}
	flow, server := newFlow(t, "", methods, storedConfigs{"telegram": "{}"})
	ctx := context.Background()

	if _, err := flow.Identify(ctx, "telegram", map[string]any{}); err != nil {
		t.Fatalf("first redemption: %v", err)
	}
	if _, err := flow.Identify(ctx, "telegram", map[string]any{}); err != nil {
		t.Fatalf("a retry within the grace window was refused: %v", err)
	}
	// The flow's clock decides the window, so date the first use back.
	if err := server.Set("auth:telegram_callback:fingerprint", fmt.Sprint(time.Now().Add(-2*replayGrace).Unix())); err != nil {
		t.Fatal(err)
	}
	_, err := flow.Identify(ctx, "telegram", map[string]any{})
	assertCode(t, err, xerr.OAuthCallbackReplayed)
}

// Telegram delivers the signed widget result to the redirect, so an
// attacker-supplied target would hand them a victim's credential; the Apple
// callback redirects the browser to it.
func TestPinnedMethodsKeepTheRedirectOnTheSiteHost(t *testing.T) {
	configs := storedConfigs{
		"telegram": `{"bot_token":"123456:AA-secret"}`,
		"apple":    `{"client_id":"com.example.panel","redirect_url":"https://api.panel.example"}`,
	}
	for _, tt := range []struct {
		name, siteHost, redirect string
		wantOK                   bool
	}{
		{name: "same host", siteHost: "panel.example.com", redirect: "https://panel.example.com/oauth", wantOK: true},
		{name: "subdomain", siteHost: "example.com", redirect: "https://panel.example.com/oauth", wantOK: true},
		{name: "site host as url", siteHost: "https://panel.example.com", redirect: "https://panel.example.com/oauth", wantOK: true},
		{name: "unpinned deployment", siteHost: "", redirect: "https://panel.example.com/oauth", wantOK: true},
		{name: "foreign host", siteHost: "panel.example.com", redirect: "https://evil.example.net/steal"},
		{name: "lookalike suffix", siteHost: "example.com", redirect: "https://evilexample.com/steal"},
		{name: "non-web scheme", siteHost: "panel.example.com", redirect: "javascript:alert(1)"},
	} {
		for _, method := range []string{"telegram", "apple"} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				flow, _ := newFlow(t, tt.siteHost, oauthprovider.Default(), configs)
				uri, err := flow.AuthURL(context.Background(), method, tt.redirect)
				if !tt.wantOK {
					assertCode(t, err, xerr.InvalidParams)
					return
				}
				if err != nil {
					t.Fatalf("AuthURL() error = %v", err)
				}
				if strings.Contains(uri, "AA-secret") {
					t.Fatalf("url %q leaks the bot token secret", uri)
				}
				parsed, err := url.Parse(uri)
				if err != nil {
					t.Fatal(err)
				}
				if method == "telegram" && parsed.Query().Get("return_to") != tt.redirect {
					t.Fatalf("return_to = %q, want %q", parsed.Query().Get("return_to"), tt.redirect)
				}
				if method == "apple" && parsed.Query().Get("state") == "" {
					t.Fatalf("apple url %q carries no state", uri)
				}
			})
		}
	}
}

// A malformed bot token is a misconfiguration, not an empty URL.
func TestTelegramWithAMalformedBotTokenIsMisconfigured(t *testing.T) {
	flow, _ := newFlow(t, "panel.example.com", oauthprovider.Default(), storedConfigs{"telegram": `{"bot_token":"no-colon-token"}`})
	_, err := flow.AuthURL(context.Background(), "telegram", "https://panel.example.com/oauth")
	assertCode(t, err, xerr.OAuthProviderMisconfigured)
	flow, _ = newFlow(t, "", oauthprovider.Default(), storedConfigs{"telegram": `{`})
	_, err = flow.AuthURL(context.Background(), "telegram", "https://panel.example.com/oauth")
	assertCode(t, err, xerr.OAuthProviderMisconfigured)
}
