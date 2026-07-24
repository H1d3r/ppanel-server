// Package identity is the facade of the identity module (accounts, auth
// methods, devices; the authentication flows join as migration proceeds).
// See docs/adr-001-modular-monolith.md.
package identity

import (
	"context"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/module/identity/internal/adminuser"
	"github.com/perfect-panel/server/internal/repository"
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
}

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
}

func New(deps Deps) Service {
	return &service{
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
	}
}

type service struct {
	adminUsers *adminuser.Service
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
