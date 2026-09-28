package bootstrap

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// Stored settings categories, as the system table and the admin settings
// handlers name them.
const (
	categorySite       = "site"
	categoryInvite     = "invite"
	categoryRegister   = "register"
	categorySubscribe  = "subscribe"
	categoryVerify     = "verify"
	categoryVerifyCode = "verify_code"
	categoryNode       = "server"
	categoryCurrency   = "currency"
)

// readSettings reads one settings category and decodes it into target. A
// read failure is returned; a stored value that cannot be applied is only
// logged with its key, because the silent decoder this replaced applied the
// remaining settings as well.
func readSettings(category string, read func(context.Context) ([]*system.System, error), target any) error {
	entries, err := read(context.Background())
	if err != nil {
		return wrapf(err, xerr.DatabaseQueryError, "read %s settings", category)
	}
	decodeSettings(category, entries, target)
	return nil
}

func decodeSettings[T config.SystemConfigEntry](category string, entries []T, target any) {
	if err := config.DecodeSystemConfig(entries, target); err != nil {
		logger.Errorw("[Settings] stored settings could not be applied, the affected fields keep their zero value",
			logger.Field("category", category), logger.Field("error", err.Error()))
	}
}

func Site(ctx *Dependencies) error {
	logger.Debug("initialize site config")
	var siteConfig config.SiteConfig
	if err := readSettings(categorySite, ctx.Store.System().GetSiteConfig, &siteConfig); err != nil {
		return err
	}
	ctx.updateConfig(func(current *config.Config) { current.Site = siteConfig })
	return nil
}

func Invite(ctx *Dependencies) error {
	logger.Debug("Invite config initialization")
	var inviteConfig config.InviteConfig
	if err := readSettings(categoryInvite, ctx.Store.System().GetInviteConfig, &inviteConfig); err != nil {
		return err
	}
	ctx.updateConfig(func(current *config.Config) { current.Invite = inviteConfig })
	return nil
}

func Register(ctx *Dependencies) error {
	logger.Debug("Register config initialization")
	var registerConfig config.RegisterConfig
	if err := readSettings(categoryRegister, ctx.Store.System().GetRegisterConfig, &registerConfig); err != nil {
		return err
	}
	ctx.updateConfig(func(current *config.Config) { current.Register = registerConfig })
	return nil
}

func Subscribe(svc *Dependencies) error {
	logger.Debug("Subscribe config initialization")
	var subscribeConfig config.SubscribeConfig
	if err := readSettings(categorySubscribe, svc.Store.System().GetSubscribeConfig, &subscribeConfig); err != nil {
		return err
	}
	svc.updateConfig(func(current *config.Config) { current.Subscribe = subscribeConfig })
	return nil
}

type verifyConfig struct {
	TurnstileSiteKey          string
	TurnstileSecret           string
	EnableLoginVerify         bool
	EnableRegisterVerify      bool
	EnableResetPasswordVerify bool
}

// Verify loads the captcha and the verification-code settings. Both are read
// before either is published, so a failed read keeps the previous pair.
func Verify(svc *Dependencies) error {
	logger.Debug("Verify config initialization")
	var verify verifyConfig
	if err := readSettings(categoryVerify, svc.Store.System().GetVerifyConfig, &verify); err != nil {
		return err
	}
	verifyConfig := config.Verify{
		TurnstileSiteKey:    verify.TurnstileSiteKey,
		TurnstileSecret:     verify.TurnstileSecret,
		LoginVerify:         verify.EnableLoginVerify,
		RegisterVerify:      verify.EnableRegisterVerify,
		ResetPasswordVerify: verify.EnableResetPasswordVerify,
	}

	logger.Debug("Verify code config initialization")
	cfg, err := svc.Store.System().GetVerifyCodeConfig(context.Background())
	if err != nil {
		return wrapf(err, xerr.DatabaseQueryError, "read %s settings", categoryVerifyCode)
	}
	verifyCode := verifyCodeFromSettings(cfg)
	svc.updateConfig(func(current *config.Config) {
		current.Verify = verifyConfig
		current.VerifyCode = verifyCode
	})
	return nil
}

// verifyCodeSettings mirrors the stored keys, which carry a VerifyCode prefix
// that the runtime config's field names do not; reflecting straight into
// config.VerifyCode matched nothing and silently ignored the admin settings.
type verifyCodeSettings struct {
	VerifyCodeExpireTime int64
	VerifyCodeLimit      int64
	VerifyCodeInterval   int64
}

func verifyCodeFromSettings[T config.SystemConfigEntry](settings []T) config.VerifyCode {
	var stored verifyCodeSettings
	decodeSettings(categoryVerifyCode, settings, &stored)
	return config.VerifyCode{
		ExpireTime: stored.VerifyCodeExpireTime,
		Limit:      stored.VerifyCodeLimit,
		Interval:   stored.VerifyCodeInterval,
	}
}
