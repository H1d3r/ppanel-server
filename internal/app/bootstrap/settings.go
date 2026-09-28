package bootstrap

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger"
)

func Site(ctx *Dependencies) {
	logger.Debug("initialize site config")
	configs, err := ctx.Store.System().GetSiteConfig(context.Background())
	if err != nil {
		panic(err)
	}
	var siteConfig config.SiteConfig
	config.SystemConfigSliceReflectToStruct(configs, &siteConfig)
	ctx.updateConfig(func(current *config.Config) { current.Site = siteConfig })
}

func Invite(ctx *Dependencies) {
	// Initialize the system configuration
	logger.Debug("Register config initialization")
	configs, err := ctx.Store.System().GetInviteConfig(context.Background())
	if err != nil {
		logger.Error("[Init Invite Config] Get Invite Config Error: ", logger.Field("error", err.Error()))
		return
	}
	var inviteConfig config.InviteConfig
	config.SystemConfigSliceReflectToStruct(configs, &inviteConfig)
	ctx.updateConfig(func(current *config.Config) { current.Invite = inviteConfig })
}

func Register(ctx *Dependencies) {
	logger.Debug("Register config initialization")
	configs, err := ctx.Store.System().GetRegisterConfig(context.Background())
	if err != nil {
		logger.Errorf("[Init Register Config] Get Register Config Error: %s", err.Error())
		return
	}
	var registerConfig config.RegisterConfig
	config.SystemConfigSliceReflectToStruct(configs, &registerConfig)
	ctx.updateConfig(func(current *config.Config) { current.Register = registerConfig })
}

func Subscribe(svc *Dependencies) {
	logger.Debug("Subscribe config initialization")
	configs, err := svc.Store.System().GetSubscribeConfig(context.Background())
	if err != nil {
		logger.Error("[Init Subscribe Config] Get Subscribe Config Error: ", logger.Field("error", err.Error()))
		return
	}

	var subscribeConfig config.SubscribeConfig
	config.SystemConfigSliceReflectToStruct(configs, &subscribeConfig)
	svc.updateConfig(func(current *config.Config) { current.Subscribe = subscribeConfig })
}

type verifyConfig struct {
	TurnstileSiteKey          string
	TurnstileSecret           string
	EnableLoginVerify         bool
	EnableRegisterVerify      bool
	EnableResetPasswordVerify bool
}

func Verify(svc *Dependencies) {
	logger.Debug("Verify config initialization")
	configs, err := svc.Store.System().GetVerifyConfig(context.Background())
	if err != nil {
		logger.Error("[Init Verify Config] Get Verify Config Error: ", logger.Field("error", err.Error()))
		return
	}
	var verify verifyConfig
	config.SystemConfigSliceReflectToStruct(configs, &verify)
	verifyConfig := config.Verify{
		TurnstileSiteKey:    verify.TurnstileSiteKey,
		TurnstileSecret:     verify.TurnstileSecret,
		LoginVerify:         verify.EnableLoginVerify,
		RegisterVerify:      verify.EnableRegisterVerify,
		ResetPasswordVerify: verify.EnableResetPasswordVerify,
	}
	svc.updateConfig(func(current *config.Config) { current.Verify = verifyConfig })

	logger.Debug("Verify code config initialization")

	cfg, err := svc.Store.System().GetVerifyCodeConfig(context.Background())
	if err != nil {
		logger.Errorf("[Init Verify Config] Get Verify Code Config Error: %s", err.Error())
		return
	}
	verifyCode := verifyCodeFromSettings(cfg)
	svc.updateConfig(func(current *config.Config) { current.VerifyCode = verifyCode })
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
	config.SystemConfigSliceReflectToStruct(settings, &stored)
	return config.VerifyCode{
		ExpireTime: stored.VerifyCodeExpireTime,
		Limit:      stored.VerifyCodeLimit,
		Interval:   stored.VerifyCodeInterval,
	}
}
