package oauth

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/internal/identitytest"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthstate"
	"github.com/perfect-panel/server/pkg/xerr"
)

// These tests run the handlers over the real identity facade, so redirect
// targets are validated as in production (oauthstate.ValidateRedirect) and
// states issued and read back from Redis: they pin what the browser sees of
// that validation.

// siteHost is the configured site host the pinned OAuth redirects must stay
// on, and the Apple callback's fallback target.
const siteHost = "https://panel.example"

// appleConfig is the stored Apple configuration: the authorization URL
// sends Apple's form post to redirect_url's callback route.
const appleConfig = `{"client_id":"com.example.panel","redirect_url":"https://api.panel.example"}`

// newFacade builds the identity facade over the module's test store with
// Apple and GitHub sign-in enabled.
func newFacade(t *testing.T) (identity.Service, *identitytest.Env) {
	t.Helper()
	env := identitytest.New(t)
	env.EnableMethod(t, "apple", appleConfig)
	env.EnableMethod(t, "github", "{}")
	return identity.New(identity.Deps{
		Users: env.Store.User(), UserAuths: env.Store.UserAuth(), Devices: env.Store.UserDevice(),
		Cache: env.Store.UserCache(), Logs: env.Store.Log(), Auths: env.Store.Auth(), Store: env.Store, Redis: env.Redis,
		AuthConfig: func() identity.AuthSnapshot { return identity.AuthSnapshot{SiteHost: siteHost} },
	}), env
}

// startLogin runs the login for method and redirect and returns the
// authorization URL it answered with.
func startLogin(t *testing.T, facade oauthFacade, method, redirect string) *url.URL {
	t.Helper()
	body := `{"method":"` + method + `","redirect":"` + redirect + `"}`
	var data dto.OAuthLoginResponse
	assertSuccess(t, post(newRouter(t, facade), loginPath, jsonBody, body), &data)
	authorize, err := url.Parse(data.Redirect)
	if err != nil {
		t.Fatalf("authorization URL %q: %v", data.Redirect, err)
	}
	return authorize
}

// The browser's whole Apple sign-in through the two handlers: the login
// answers with Apple's authorization URL carrying a fresh state, and Apple's
// form post of that state and a code is redirected, with 302, to the page
// the login named, carrying the code and state on to the token exchange.
func TestAppleSignInRedirectsTheFormPostToThePageTheLoginNamed(t *testing.T) {
	facade, _ := newFacade(t)
	authorize := startLogin(t, facade, "apple", "https://panel.example/oauth/apple")
	query := authorize.Query()
	state := query.Get("state")
	if authorize.Scheme != "https" || authorize.Host != "appleid.apple.com" || authorize.Path != "/auth/authorize" || state == "" ||
		query.Get("redirect_uri") != "https://api.panel.example/v1/auth/oauth/callback/apple" || query.Get("response_mode") != "form_post" {
		t.Fatalf("authorization URL = %s", authorize)
	}

	w := post(newRouter(t, facade), appleCallbackPath, formPost,
		url.Values{"code": {"apple-code"}, "id_token": {"apple-id-token"}, "state": {state}}.Encode())
	want := "https://panel.example/oauth/apple?" + url.Values{"code": {"apple-code"}, "method": {"apple"}, "state": {state}}.Encode()
	if w.Code != http.StatusFound || w.Header().Get("Location") != want {
		t.Fatalf("callback answer = %d to %q, want 302 to %q", w.Code, w.Header().Get("Location"), want)
	}
}

// A redirect the callback would send the browser to must stay on the site
// host or one of its subdomains and be a web URL; the login refuses any
// other before it issues a state. A code-flow provider's redirect is left
// to the provider, which only returns to the redirect URI registered with
// it.
func TestOAuthLoginPinsTheRedirectOfMethodsThatRedirectTheBrowser(t *testing.T) {
	for _, tc := range []struct {
		method, redirect string
		allowed          bool
	}{
		{"apple", "https://panel.example/oauth/apple", true},
		{"apple", "https://app.panel.example/oauth/apple", true},
		{"apple", "http://panel.example/oauth/apple", true},
		{"apple", "https://evil.example/phish", false},
		{"apple", "https://panel.example.evil.example/phish", false},
		{"apple", "https://evilpanel.example/phish", false},
		{"apple", "https://panel.example@evil.example/phish", false},
		{"apple", "//evil.example/phish", false},
		{"apple", "javascript:alert(1)", false},
		{"apple", "", false},
		{"github", "https://evil.example/oauth/github", true},
	} {
		t.Run(tc.method+" "+tc.redirect, func(t *testing.T) {
			facade, env := newFacade(t)
			body := `{"method":"` + tc.method + `","redirect":"` + tc.redirect + `"}`
			w := post(newRouter(t, facade), loginPath, jsonBody, body)
			if !tc.allowed {
				assertFailure(t, w, xerr.InvalidParams, "Param Error")
				if keys := env.Mini.Keys(); len(keys) != 0 {
					t.Fatalf("a refused redirect stored %v", keys)
				}
				return
			}
			var data dto.OAuthLoginResponse
			assertSuccess(t, w, &data)
			if len(env.Mini.Keys()) != 1 {
				t.Fatalf("states = %v, want the one issued with the URL", env.Mini.Keys())
			}
		})
	}
}

// A method the administrator has not enabled cannot start a sign-in.
func TestOAuthLoginRefusesAMethodThatIsNotEnabled(t *testing.T) {
	facade, env := newFacade(t)
	w := post(newRouter(t, facade), loginPath, jsonBody, `{"method":"google","redirect":"https://panel.example/oauth/google"}`)
	assertFailure(t, w, xerr.GetAuthenticatorError, "Unsupported login method")
	if keys := env.Mini.Keys(); len(keys) != 0 {
		t.Fatalf("a refused login stored %v", keys)
	}
}

// Apple's form post of a state the server does not hold (never issued,
// expired or already redeemed), or of one whose stored redirect left the
// site host, sends the browser back to the site host with 307 and nothing
// of the callback.
func TestAppleCallbackFallsBackToTheSiteHost(t *testing.T) {
	facade, env := newFacade(t)
	planted, err := oauthstate.Issue(context.Background(), env.Redis, "apple", oauthstate.LoginScope(), "https://evil.example/phish")
	if err != nil {
		t.Fatal(err)
	}
	for name, state := range map[string]string{"unknown state": "never-issued", "redirect off the site host": planted} {
		t.Run(name, func(t *testing.T) {
			w := post(newRouter(t, facade), appleCallbackPath, formPost, url.Values{"code": {"apple-code"}, "state": {state}}.Encode())
			if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != siteHost {
				t.Fatalf("callback answer = %d to %q, want 307 to %q", w.Code, w.Header().Get("Location"), siteHost)
			}
		})
	}
}

// The token exchange redeems the state the login issued, reading the code
// and state from the callback object: a state the server does not hold is
// refused, and so is a callback that is not an object.
func TestOAuthLoginGetTokenRefusesACallbackWithoutAnIssuedState(t *testing.T) {
	facade, _ := newFacade(t)
	for name, tc := range map[string]struct {
		body string
		code uint32
		msg  string
	}{
		"unknown state": {`{"method":"github","callback":{"code":"github-code","state":"never-issued"}}`, xerr.OAuthStateInvalid, "OAuth state is invalid or expired"},
		"no state":      {`{"method":"github","callback":{"code":"github-code"}}`, xerr.InvalidParams, "Param Error"},
		"not an object": {`{"method":"github","callback":"github-code"}`, xerr.InvalidParams, "Param Error"},
	} {
		t.Run(name, func(t *testing.T) {
			assertFailure(t, post(newRouter(t, facade), tokenPath, jsonBody, tc.body), tc.code, tc.msg)
		})
	}
}
