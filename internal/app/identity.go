package app

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

// newIdentityModule wires the identity module against the shared store;
// device kicking is a closure over the application's device manager.
// The trial settings are not passed: the subscription module grants trials
// when it consumes the registration event.
func newIdentityModule(store repository.Store, srv *Application) identity.Service {
	return identity.New(identity.Deps{
		Users:     store.User(),
		UserAuths: store.UserAuth(),
		Devices:   store.UserDevice(),
		Cache:     store.UserCache(),
		Logs:      store.Log(),
		Store:     store,
		KickDevice: func(userID int64, identifier string) {
			if srv.DeviceManager != nil {
				srv.DeviceManager.KickDevice(userID, identifier)
			}
		},
		// Deleted and disabled accounts lose their cached access at once.
		SubscriptionCaches: srv.Subscription,
		ServerCaches:       identityServerCaches{srv},

		// Billing is constructed before identity, so its facade is bound
		// directly.
		Wallet: srv.Billing,
		Auths:  store.Auth(),
		Redis:  srv.Redis,
		EmailDomains: func() (string, bool) {
			current := srv.Runtime.Config().Email
			return current.DomainSuffixList, current.EnableDomainSuffix
		},
		TelegramBotName: func() string { return srv.Runtime.Config().Telegram.BotName },
		NotifyTelegramUnbind: func(ctx context.Context, userID, chatID int64) error {
			return srv.Notification.NotifyTelegramUnbind(ctx, userID, chatID)
		},
		AuthConfig: func() identity.AuthSnapshot {
			c := srv.Runtime.Config()
			return identity.AuthSnapshot{
				JWTAccessSecret: c.JwtAuth.AccessSecret,
				JWTAccessExpire: c.JwtAuth.AccessExpire,

				EmailEnabled:            c.Email.Enable,
				EmailVerifyEnabled:      c.Email.EnableVerify,
				EmailDomainSuffixList:   c.Email.DomainSuffixList,
				EmailEnableDomainSuffix: c.Email.EnableDomainSuffix,
				MobileEnabled:           c.Mobile.Enable,
				DeviceEnabled:           c.Device.Enable,
				DeviceOnlyReal:          c.Device.OnlyRealDevice,

				InviteForced:      c.Invite.ForcedInvite,
				OnlyFirstPurchase: c.Invite.OnlyFirstPurchase,

				StopRegister:            c.Register.StopRegister,
				RegisterVerify:          c.Verify.RegisterVerify,
				LoginVerify:             c.Verify.LoginVerify,
				ResetPasswordVerify:     c.Verify.ResetPasswordVerify,
				TurnstileSecret:         c.Verify.TurnstileSecret,
				EnableIpRegisterLimit:   c.Register.EnableIpRegisterLimit,
				IpRegisterLimit:         c.Register.IpRegisterLimit,
				IpRegisterLimitDuration: c.Register.IpRegisterLimitDuration,

				SiteHost: c.Site.Host,
			}
		},
		VerifyQueue: srv.Queue,
		SenderConfig: func() identity.SenderSnapshot {
			c := srv.Runtime.Config()
			return identity.SenderSnapshot{
				EmailPlatform:        c.Email.Platform,
				EmailPlatformConfig:  c.Email.PlatformConfig,
				MobilePlatform:       c.Mobile.Platform,
				MobilePlatformConfig: c.Mobile.PlatformConfig,
				SiteName:             c.Site.SiteName,
			}
		},
		Reinitialize: srv.Runtime.Reinitialize,
		VerifyCodeConfig: func() identity.VerifyCodeSnapshot {
			c := srv.Runtime.Config()
			return identity.VerifyCodeSnapshot{
				DomainSuffixList:       c.Email.DomainSuffixList,
				EnableDomainSuffix:     c.Email.EnableDomainSuffix,
				VerifyCodeInterval:     c.VerifyCode.Interval,
				VerifyCodeLimit:        c.VerifyCode.Limit,
				VerifyCodeExpire:       c.VerifyCode.ExpireTime,
				MobileWhitelistEnabled: c.Mobile.EnableWhitelist,
				MobileWhitelist:        c.Mobile.Whitelist,
				SiteLogo:               c.Site.SiteLogo,
				SiteName:               c.Site.SiteName,
			}
		},
	})
}

// normalizeIdentityData runs the identity module's idempotent startup data
// fix-ups once the schema is current; the module logs what they changed.
// They repair stored identifiers, so a failure is logged and the server still
// starts.
func normalizeIdentityData(ctx context.Context, accounts IdentityStartup) {
	if err := accounts.NormalizePhoneNumbers(ctx); err != nil {
		logger.Errorw("[Identity] normalize stored phone numbers failed", logger.Field("error", err.Error()))
	}
}
