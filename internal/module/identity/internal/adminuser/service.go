// Package adminuser implements the admin-side account management subdomain
// of the identity module: user CRUD, auth methods, devices and login logs.
// Only the module facade may reach it.
package adminuser

import (
	"context"
	"fmt"
	"os"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

type Deps struct {
	Users     repository.UserRepo
	UserAuths repository.UserAuthRepo
	Devices   repository.UserDeviceRepo
	Cache     repository.UserCacheRepo
	// UserSubs and Logs are read ports onto the subscription and platform
	// domains: the deletion cache cascade and the login logs.
	UserSubs repository.UserSubscriptionRepo
	Logs     repository.LogRepo
	// Wallet is the read port onto the billing domain: the admin views show
	// wallet values from the authoritative table, not the legacy user
	// columns (ADR-001 step 5).
	Wallet repository.WalletRepo
	Store  Store
	Redis  *redis.Client
	// KickDevice force-disconnects a bound device.
	KickDevice func(userID int64, identifier string)
}

func (d Deps) kickDevice(userID int64, identifier string) {
	if d.KickDevice != nil {
		d.KickDevice(userID, identifier)
	}
}

// demoAdminID is the administrator account of the public demo instance.
const demoAdminID = 2

// demoMode reports whether this instance is the public demo, whose
// administrator must stay usable.
func demoMode() bool {
	return strings.EqualFold(os.Getenv("PPANEL_MODE"), "demo")
}

func demoRestricted(operation string) error {
	return fmt.Errorf("demo mode does not allow to %s: %w", operation, xerr.NewErrCode(xerr.DemoModeRestricted))
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) DeleteUser(ctx context.Context, req *dto.GetDetailRequest) error {
	return newDeleteUserLogic(ctx, s.deps).DeleteUser(req)
}

func (s *Service) BatchDeleteUser(ctx context.Context, req *dto.BatchDeleteUserRequest) error {
	return newBatchDeleteUserLogic(ctx, s.deps).BatchDeleteUser(req)
}

func (s *Service) GetUserDetail(ctx context.Context, req *dto.GetDetailRequest) (*dto.User, error) {
	return newGetUserDetailLogic(ctx, s.deps).GetUserDetail(req)
}

func (s *Service) GetUserList(ctx context.Context, req *dto.GetUserListRequest) (*dto.GetUserListResponse, error) {
	return newGetUserListLogic(ctx, s.deps).GetUserList(req)
}

func (s *Service) CurrentUser(ctx context.Context) (*dto.User, error) {
	return newCurrentUserLogic(ctx, s.deps).CurrentUser()
}

func (s *Service) DeleteUserAuthMethod(ctx context.Context, req *dto.DeleteUserAuthMethodRequest) error {
	return newDeleteUserAuthMethodLogic(ctx, s.deps).DeleteUserAuthMethod(req)
}

func (s *Service) GetUserAuthMethod(ctx context.Context, req *dto.GetUserAuthMethodRequest) (*dto.GetUserAuthMethodResponse, error) {
	return newGetUserAuthMethodLogic(ctx, s.deps).GetUserAuthMethod(req)
}

func (s *Service) DeleteUserDevice(ctx context.Context, req *dto.DeleteUserDeviceRequest) error {
	return newDeleteUserDeviceLogic(ctx, s.deps).DeleteUserDevice(req)
}

func (s *Service) UpdateUserDevice(ctx context.Context, req *dto.UserDevice) error {
	return newUpdateUserDeviceLogic(ctx, s.deps).UpdateUserDevice(req)
}

func (s *Service) KickOfflineByUserDevice(ctx context.Context, req *dto.KickOfflineRequest) error {
	return newKickOfflineByUserDeviceLogic(ctx, s.deps).KickOfflineByUserDevice(req)
}

func (s *Service) GetUserLoginLogs(ctx context.Context, req *dto.GetUserLoginLogsRequest) (*dto.GetUserLoginLogsResponse, error) {
	return newGetUserLoginLogsLogic(ctx, s.deps).GetUserLoginLogs(req)
}

func (s *Service) UpdateUserNotifySetting(ctx context.Context, req *dto.UpdateUserNotifySettingRequest) error {
	return newUpdateUserNotifySettingLogic(ctx, s.deps).UpdateUserNotifySetting(req)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.BillingTransactor
	repository.IdentityTransactor
	Node() repository.NodeRepo
}
