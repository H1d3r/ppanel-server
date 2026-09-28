package authmethodadmin

import (
	"context"
	"encoding/json"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/identitytest"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/xerr"
)

type fixture struct {
	*identitytest.Env
	svc       *Service
	reloaded  []string
	senderCfg Snapshot
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	logtest.Discard(t)
	env := identitytest.New(t)
	f := &fixture{Env: env}
	f.svc = NewService(Deps{
		Auths:        env.Store.Auth(),
		Config:       func() Snapshot { return f.senderCfg },
		Reinitialize: func(subsystem string) { f.reloaded = append(f.reloaded, subsystem) },
	})
	for _, method := range []string{"email", "mobile", "device", "github"} {
		env.EnableMethod(t, method, "{}")
	}
	return f
}

func (f *fixture) stored(t *testing.T, method string) string {
	t.Helper()
	var row auth.Auth
	if err := f.DB.Where("method = ?", method).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.Config
}

// request is an update of method as the admin panel sends it: with the
// method's id and switch.
func (f *fixture) request(t *testing.T, method string, config any) *dto.UpdateAuthMethodConfigRequest {
	t.Helper()
	var row auth.Auth
	if err := f.DB.Where("method = ?", method).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	return &dto.UpdateAuthMethodConfigRequest{Id: row.Id, Method: method, Config: config, Enabled: row.Enabled}
}

func assertCode(t *testing.T, err error, want uint32) {
	t.Helper()
	if got := xerr.CodeOf(err); err == nil || got != want {
		t.Fatalf("error = %v (code %d), want code %d", err, got, want)
	}
}

// A configuration that does not decode is refused and the stored one kept;
// it used to be replaced by the defaults, wiping the sender settings.
func TestUpdateAuthMethodConfigRefusesConfigsThatDoNotDecode(t *testing.T) {
	f := newFixture(t)
	for method, config := range map[string]any{
		"email":  map[string]any{"enable_verify": "yes"},
		"mobile": map[string]any{"whitelist": "86"},
		"device": map[string]any{"enable_security": true},
		"github": "not an object",
	} {
		_, err := f.svc.UpdateAuthMethodConfig(context.Background(), f.request(t, method, config))
		assertCode(t, err, xerr.InvalidParams)
		if got := f.stored(t, method); got != "{}" {
			t.Fatalf("%s config = %s, want it unchanged", method, got)
		}
	}
	if len(f.reloaded) != 0 {
		t.Fatalf("reloaded %v after refused updates", f.reloaded)
	}
}

func TestUpdateAuthMethodConfigStoresAndReloads(t *testing.T) {
	f := newFixture(t)
	resp, err := f.svc.UpdateAuthMethodConfig(context.Background(), f.request(t, "mobile",
		map[string]any{"platform": "twilio", "enable_whitelist": true, "whitelist": []any{"86"}}))
	if err != nil {
		t.Fatalf("UpdateAuthMethodConfig() error = %v", err)
	}
	var stored auth.MobileAuthConfig
	if err := json.Unmarshal([]byte(f.stored(t, "mobile")), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Platform != "twilio" || !stored.EnableWhitelist || len(stored.Whitelist) != 1 {
		t.Fatalf("stored = %+v", stored)
	}
	if config, ok := resp.Config.(map[string]any); !ok || config["platform"] != "twilio" {
		t.Fatalf("response config = %#v", resp.Config)
	}
	if len(f.reloaded) != 1 || f.reloaded[0] != "mobile" {
		t.Fatalf("reloaded = %v, want [mobile]", f.reloaded)
	}

	// A provider method keeps what the administrator sent and needs no
	// reload.
	if _, err := f.svc.UpdateAuthMethodConfig(context.Background(), f.request(t, "github", map[string]any{"client_id": "github-id"})); err != nil {
		t.Fatal(err)
	}
	if got := f.stored(t, "github"); got != `{"client_id":"github-id"}` {
		t.Fatalf("github config = %s", got)
	}
	if len(f.reloaded) != 1 {
		t.Fatalf("reloaded = %v, want no reload for github", f.reloaded)
	}
}

// Without a configuration the method returns to its defaults.
func TestUpdateAuthMethodConfigWithoutConfigResetsTheDefaults(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.UpdateAuthMethodConfig(context.Background(), f.request(t, "email", nil)); err != nil {
		t.Fatal(err)
	}
	var stored auth.EmailAuthConfig
	if err := json.Unmarshal([]byte(f.stored(t, "email")), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.VerifyEmailTemplate == "" || stored.VerifyEmailSubject == "" {
		t.Fatalf("stored = %+v, want the default templates", stored)
	}
}

func TestDeviceConfigNeedsItsSecurityForRealDevices(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.UpdateAuthMethodConfig(context.Background(), f.request(t, "device", map[string]any{"only_real_device": true}))
	assertCode(t, err, xerr.InvalidParams)
	if _, err := f.svc.UpdateAuthMethodConfig(context.Background(), f.request(t, "device",
		map[string]any{"only_real_device": true, "enable_security": true, "security_secret": "key"})); err != nil {
		t.Fatalf("a complete device config was refused: %v", err)
	}
}

func TestAuthMethodListAndConfigDecodeTheStoredConfig(t *testing.T) {
	f := newFixture(t)
	if err := f.DB.Model(&auth.Auth{}).Where("method = ?", "github").Update("config", `{"client_id":"github-id"}`).Error; err != nil {
		t.Fatal(err)
	}
	config, err := f.svc.GetAuthMethodConfig(context.Background(), &dto.GetAuthMethodConfigRequest{Method: "github"})
	if err != nil {
		t.Fatal(err)
	}
	if decoded, ok := config.Config.(map[string]any); !ok || decoded["client_id"] != "github-id" {
		t.Fatalf("config = %#v", config.Config)
	}
	list, err := f.svc.GetAuthMethodList(context.Background())
	if err != nil || len(list.List) != 4 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if err := f.DB.Model(&auth.Auth{}).Where("method = ?", "github").Update("config", `{`).Error; err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.GetAuthMethodConfig(context.Background(), &dto.GetAuthMethodConfigRequest{Method: "github"})
	assertCode(t, err, xerr.ERROR)
	_, err = f.svc.GetAuthMethodConfig(context.Background(), &dto.GetAuthMethodConfigRequest{Method: "linkedin"})
	assertCode(t, err, xerr.DatabaseQueryError)
}

// A test send that fails tells the administrator why.
func TestTestSendReportsTheSenderFailure(t *testing.T) {
	f := newFixture(t)
	f.senderCfg = Snapshot{EmailPlatform: "no-such-platform", MobilePlatform: "no-such-platform"}
	err := f.svc.TestEmailSend(context.Background(), &dto.TestEmailSendRequest{Email: "admin@example.com"})
	if err == nil {
		t.Fatal("TestEmailSend() with an unknown platform succeeded")
	}
	err = f.svc.TestSmsSend(context.Background(), &dto.TestSmsSendRequest{AreaCode: "86", Telephone: "13800138000"})
	if err == nil {
		t.Fatal("TestSmsSend() with an unknown platform succeeded")
	}
}
