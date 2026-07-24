// Package identity is the facade of the identity module (accounts, auth
// methods, devices; the authentication flows join as migration proceeds).
// See docs/adr-001-modular-monolith.md.
package identity

import (
	"context"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/module/identity/internal/adminuser"
	authn "github.com/perfect-panel/server/internal/module/identity/internal/authn"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/oauth"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/module/identity/internal/profile"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Service is the only surface other code may depend on; the implementation
// lives under internal/ where the compiler seals it off.
type Service interface {
	CreateUser(ctx context.Context, req *dto.CreateUserRequest) error
	DeleteUser(ctx context.Context, req *dto.GetDetailRequest) error
	BatchDeleteUser(ctx context.Context, req *dto.BatchDeleteUserRequest) error
	GetUserDetail(ctx context.Context, req *dto.GetDetailRequest) (*dto.User, error)
	GetUserList(ctx context.Context, req *dto.GetUserListRequest) (*dto.GetUserListResponse, error)
	CurrentUser(ctx context.Context) (*dto.User, error)
	CreateUserAuthMethod(ctx context.Context, req *dto.CreateUserAuthMethodRequest) error
	DeleteUserAuthMethod(ctx context.Context, req *dto.DeleteUserAuthMethodRequest) error
	GetUserAuthMethod(ctx context.Context, req *dto.GetUserAuthMethodRequest) (*dto.GetUserAuthMethodResponse, error)
	UpdateUserAuthMethod(ctx context.Context, req *dto.UpdateUserAuthMethodRequest) error
	DeleteUserDevice(ctx context.Context, req *dto.DeleteUserDeivceRequest) error
	UpdateUserDevice(ctx context.Context, req *dto.UserDevice) error
	KickOfflineByUserDevice(ctx context.Context, req *dto.KickOfflineRequest) error
	GetUserLoginLogs(ctx context.Context, req *dto.GetUserLoginLogsRequest) (*dto.GetUserLoginLogsResponse, error)
	UpdateUserBasicInfo(ctx context.Context, req *dto.UpdateUserBasiceInfoRequest) error
	UpdateUserNotifySetting(ctx context.Context, req *dto.UpdateUserNotifySettingRequest) error

	// The profile flows resolve the current user from the request context:
	// account info, credentials, third-party bindings, devices and
	// notification preferences.
	QueryUserInfo(ctx context.Context) (*dto.User, error)
	UpdateUserPassword(ctx context.Context, req *dto.UpdateUserPasswordRequest) error
	UpdateUserNotify(ctx context.Context, req *dto.UpdateUserNotifyRequest) error
	UpdateUserRules(ctx context.Context, req *dto.UpdateUserRulesRequest) error
	GetLoginLog(ctx context.Context, req *dto.GetLoginLogRequest) (*dto.GetLoginLogResponse, error)
	GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error)
	UnbindDevice(ctx context.Context, req *dto.UnbindDeviceRequest) error
	GetOAuthMethods(ctx context.Context) (*dto.GetOAuthMethodsResponse, error)
	BindOAuth(ctx context.Context, req *dto.BindOAuthRequest) (*dto.BindOAuthResponse, error)
	BindOAuthCallback(ctx context.Context, req *dto.BindOAuthCallbackRequest) error
	UnbindOAuth(ctx context.Context, req *dto.UnbindOAuthRequest) error
	BindTelegram(ctx context.Context) (*dto.BindTelegramResponse, error)
	UnbindTelegram(ctx context.Context) error
	UpdateBindEmail(ctx context.Context, req *dto.UpdateBindEmailRequest) error
	VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) error
	UpdateBindMobile(ctx context.Context, req *dto.UpdateBindMobileRequest) error

	// The authentication flows: existence checks, credential/telephone/device
	// login and registration, password resets and the OAuth handshakes.
	// Transport concerns (client IP, user agent, login turnstile) stay in the
	// handlers.
	CheckUser(ctx context.Context, req *dto.CheckUserRequest) (*dto.CheckUserResponse, error)
	CheckUserTelephone(ctx context.Context, req *dto.TelephoneCheckUserRequest) (*dto.TelephoneCheckUserResponse, error)
	UserLogin(ctx context.Context, req *dto.UserLoginRequest) (*dto.LoginResponse, error)
	UserRegister(ctx context.Context, req *dto.UserRegisterRequest) (*dto.LoginResponse, error)
	TelephoneLogin(ctx context.Context, req *dto.TelephoneLoginRequest, ip, userAgent string) (*dto.LoginResponse, error)
	TelephoneUserRegister(ctx context.Context, req *dto.TelephoneRegisterRequest) (*dto.LoginResponse, error)
	ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.LoginResponse, error)
	TelephoneResetPassword(ctx context.Context, req *dto.TelephoneResetPasswordRequest) (*dto.LoginResponse, error)
	DeviceLogin(ctx context.Context, req *dto.DeviceLoginRequest) (*dto.LoginResponse, error)
	OAuthLogin(ctx context.Context, req *dto.OAthLoginRequest) (*dto.OAuthLoginResponse, error)
	OAuthLoginGetToken(ctx context.Context, req *dto.OAuthLoginGetTokenRequest, ip, userAgent string) (*dto.LoginResponse, error)
	AppleLoginCallback(ctx context.Context, req *dto.AppleLoginCallbackRequest) (*AppleLoginRedirect, error)

	// AuthPolicy exposes the administrator-configured account policies to
	// collaborators still outside the module (the verification-code senders
	// in legacy common logic); it becomes module-internal once those flows
	// migrate.
	AuthPolicy() RegisterPolicy
}

// RegisterPolicy re-exports the register-policy implementation for
// transitional out-of-module consumers.
type RegisterPolicy = registerpolicy.ServicePolicy

// AuthSnapshot re-exports the authentication subdomain's per-request view of
// the runtime-mutable settings; the composition root supplies the snapshot
// closure.
type AuthSnapshot = authn.Snapshot

// AppleLoginRedirect re-exports the Apple form-post callback's redirect
// result for the transport handler.
type AppleLoginRedirect = oauth.AppleLoginRedirect

// Deps declares everything the module needs; the composition root
// (internal/svc) provides them.
type Deps struct {
	Users     repository.UserRepo
	UserAuths repository.UserAuthRepo
	Devices   repository.UserDeviceRepo
	Cache     repository.UserCacheRepo
	UserSubs  repository.UserSubscriptionRepo
	Plans     repository.SubscribeRepo
	Traffic   repository.TrafficRepo
	Logs      repository.LogRepo
	Store     repository.Store
	// KickDevice force-disconnects a bound device.
	KickDevice func(userID int64, identifier string)

	// Profile-specific dependencies.
	Auths repository.AuthRepo
	Redis *redis.Client
	// EmailDomains snapshots the runtime-mutable email domain-suffix policy.
	EmailDomains func() (domainList string, restrict bool)
	// TelegramBotName snapshots the runtime-mutable Telegram bot name.
	TelegramBotName func() string
	// NotifyTelegramUnbind sends the best-effort unbind notice.
	NotifyTelegramUnbind func(userID, chatID int64) error
	// AuthConfig snapshots the runtime-mutable settings consumed by the
	// authentication flows per request.
	AuthConfig func() AuthSnapshot
}

func New(deps Deps) Service {
	authSvc := authn.NewService(authn.Deps{
		Store:  deps.Store,
		Redis:  deps.Redis,
		Config: deps.AuthConfig,
	})
	return &service{
		authn: authSvc,
		adminUsers: adminuser.NewService(adminuser.Deps{
			Users:      deps.Users,
			UserAuths:  deps.UserAuths,
			Devices:    deps.Devices,
			Cache:      deps.Cache,
			UserSubs:   deps.UserSubs,
			Plans:      deps.Plans,
			Traffic:    deps.Traffic,
			Logs:       deps.Logs,
			Store:      deps.Store,
			KickDevice: deps.KickDevice,
		}),
		profile: profile.NewService(profile.Deps{
			Users:           deps.Users,
			UserAuth:        deps.UserAuths,
			Auth:            deps.Auths,
			Devices:         deps.Devices,
			UserCache:       deps.Cache,
			Logs:            deps.Logs,
			Redis:           deps.Redis,
			Store:           deps.Store,
			Policy:          authSvc.Policy(),
			EmailDomains:    deps.EmailDomains,
			TelegramBotName: deps.TelegramBotName,
			NotifyUnbind:    deps.NotifyTelegramUnbind,
		}),
	}
}

type service struct {
	adminUsers *adminuser.Service
	profile    *profile.Service
	authn      *authn.Service
}

func (s *service) CreateUser(ctx context.Context, req *dto.CreateUserRequest) error {
	return s.adminUsers.CreateUser(ctx, req)
}

func (s *service) DeleteUser(ctx context.Context, req *dto.GetDetailRequest) error {
	return s.adminUsers.DeleteUser(ctx, req)
}

func (s *service) BatchDeleteUser(ctx context.Context, req *dto.BatchDeleteUserRequest) error {
	return s.adminUsers.BatchDeleteUser(ctx, req)
}

func (s *service) GetUserDetail(ctx context.Context, req *dto.GetDetailRequest) (*dto.User, error) {
	return s.adminUsers.GetUserDetail(ctx, req)
}

func (s *service) GetUserList(ctx context.Context, req *dto.GetUserListRequest) (*dto.GetUserListResponse, error) {
	return s.adminUsers.GetUserList(ctx, req)
}

func (s *service) CurrentUser(ctx context.Context) (*dto.User, error) {
	return s.adminUsers.CurrentUser(ctx)
}

func (s *service) CreateUserAuthMethod(ctx context.Context, req *dto.CreateUserAuthMethodRequest) error {
	return s.adminUsers.CreateUserAuthMethod(ctx, req)
}

func (s *service) DeleteUserAuthMethod(ctx context.Context, req *dto.DeleteUserAuthMethodRequest) error {
	return s.adminUsers.DeleteUserAuthMethod(ctx, req)
}

func (s *service) GetUserAuthMethod(ctx context.Context, req *dto.GetUserAuthMethodRequest) (*dto.GetUserAuthMethodResponse, error) {
	return s.adminUsers.GetUserAuthMethod(ctx, req)
}

func (s *service) UpdateUserAuthMethod(ctx context.Context, req *dto.UpdateUserAuthMethodRequest) error {
	return s.adminUsers.UpdateUserAuthMethod(ctx, req)
}

func (s *service) DeleteUserDevice(ctx context.Context, req *dto.DeleteUserDeivceRequest) error {
	return s.adminUsers.DeleteUserDevice(ctx, req)
}

func (s *service) UpdateUserDevice(ctx context.Context, req *dto.UserDevice) error {
	return s.adminUsers.UpdateUserDevice(ctx, req)
}

func (s *service) KickOfflineByUserDevice(ctx context.Context, req *dto.KickOfflineRequest) error {
	return s.adminUsers.KickOfflineByUserDevice(ctx, req)
}

func (s *service) GetUserLoginLogs(ctx context.Context, req *dto.GetUserLoginLogsRequest) (*dto.GetUserLoginLogsResponse, error) {
	return s.adminUsers.GetUserLoginLogs(ctx, req)
}

func (s *service) UpdateUserBasicInfo(ctx context.Context, req *dto.UpdateUserBasiceInfoRequest) error {
	return s.adminUsers.UpdateUserBasicInfo(ctx, req)
}

func (s *service) UpdateUserNotifySetting(ctx context.Context, req *dto.UpdateUserNotifySettingRequest) error {
	return s.adminUsers.UpdateUserNotifySetting(ctx, req)
}

func (s *service) QueryUserInfo(ctx context.Context) (*dto.User, error) {
	return s.profile.QueryUserInfo(ctx)
}

func (s *service) UpdateUserPassword(ctx context.Context, req *dto.UpdateUserPasswordRequest) error {
	return s.profile.UpdateUserPassword(ctx, req)
}

func (s *service) UpdateUserNotify(ctx context.Context, req *dto.UpdateUserNotifyRequest) error {
	return s.profile.UpdateUserNotify(ctx, req)
}

func (s *service) UpdateUserRules(ctx context.Context, req *dto.UpdateUserRulesRequest) error {
	return s.profile.UpdateUserRules(ctx, req)
}

func (s *service) GetLoginLog(ctx context.Context, req *dto.GetLoginLogRequest) (*dto.GetLoginLogResponse, error) {
	return s.profile.GetLoginLog(ctx, req)
}

func (s *service) GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error) {
	return s.profile.GetDeviceList(ctx)
}

func (s *service) UnbindDevice(ctx context.Context, req *dto.UnbindDeviceRequest) error {
	return s.profile.UnbindDevice(ctx, req)
}

func (s *service) GetOAuthMethods(ctx context.Context) (*dto.GetOAuthMethodsResponse, error) {
	return s.profile.GetOAuthMethods(ctx)
}

func (s *service) BindOAuth(ctx context.Context, req *dto.BindOAuthRequest) (*dto.BindOAuthResponse, error) {
	return s.profile.BindOAuth(ctx, req)
}

func (s *service) BindOAuthCallback(ctx context.Context, req *dto.BindOAuthCallbackRequest) error {
	return s.profile.BindOAuthCallback(ctx, req)
}

func (s *service) UnbindOAuth(ctx context.Context, req *dto.UnbindOAuthRequest) error {
	return s.profile.UnbindOAuth(ctx, req)
}

func (s *service) BindTelegram(ctx context.Context) (*dto.BindTelegramResponse, error) {
	return s.profile.BindTelegram(ctx)
}

func (s *service) UnbindTelegram(ctx context.Context) error {
	return s.profile.UnbindTelegram(ctx)
}

func (s *service) UpdateBindEmail(ctx context.Context, req *dto.UpdateBindEmailRequest) error {
	return s.profile.UpdateBindEmail(ctx, req)
}

func (s *service) VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) error {
	return s.profile.VerifyEmail(ctx, req)
}

func (s *service) UpdateBindMobile(ctx context.Context, req *dto.UpdateBindMobileRequest) error {
	return s.profile.UpdateBindMobile(ctx, req)
}

func (s *service) CheckUser(ctx context.Context, req *dto.CheckUserRequest) (*dto.CheckUserResponse, error) {
	return s.authn.CheckUser(ctx, req)
}

func (s *service) CheckUserTelephone(ctx context.Context, req *dto.TelephoneCheckUserRequest) (*dto.TelephoneCheckUserResponse, error) {
	return s.authn.CheckUserTelephone(ctx, req)
}

func (s *service) UserLogin(ctx context.Context, req *dto.UserLoginRequest) (*dto.LoginResponse, error) {
	return s.authn.UserLogin(ctx, req)
}

func (s *service) UserRegister(ctx context.Context, req *dto.UserRegisterRequest) (*dto.LoginResponse, error) {
	return s.authn.UserRegister(ctx, req)
}

func (s *service) TelephoneLogin(ctx context.Context, req *dto.TelephoneLoginRequest, ip, userAgent string) (*dto.LoginResponse, error) {
	return s.authn.TelephoneLogin(ctx, req, ip, userAgent)
}

func (s *service) TelephoneUserRegister(ctx context.Context, req *dto.TelephoneRegisterRequest) (*dto.LoginResponse, error) {
	return s.authn.TelephoneUserRegister(ctx, req)
}

func (s *service) ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.LoginResponse, error) {
	return s.authn.ResetPassword(ctx, req)
}

func (s *service) TelephoneResetPassword(ctx context.Context, req *dto.TelephoneResetPasswordRequest) (*dto.LoginResponse, error) {
	return s.authn.TelephoneResetPassword(ctx, req)
}

func (s *service) DeviceLogin(ctx context.Context, req *dto.DeviceLoginRequest) (*dto.LoginResponse, error) {
	return s.authn.DeviceLogin(ctx, req)
}

func (s *service) OAuthLogin(ctx context.Context, req *dto.OAthLoginRequest) (*dto.OAuthLoginResponse, error) {
	return s.authn.OAuthLogin(ctx, req)
}

func (s *service) OAuthLoginGetToken(ctx context.Context, req *dto.OAuthLoginGetTokenRequest, ip, userAgent string) (*dto.LoginResponse, error) {
	return s.authn.OAuthLoginGetToken(ctx, req, ip, userAgent)
}

func (s *service) AppleLoginCallback(ctx context.Context, req *dto.AppleLoginCallbackRequest) (*AppleLoginRedirect, error) {
	return s.authn.AppleLoginCallback(ctx, req)
}

func (s *service) AuthPolicy() RegisterPolicy {
	return s.authn.Policy()
}
