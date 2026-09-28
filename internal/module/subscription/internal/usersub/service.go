// Package usersub implements the admin-side user subscription management of
// the subscription module. Only the module facade may reach it.
package usersub

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	trafficEntity "github.com/perfect-panel/server/internal/module/network/entity/traffic"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
)

// The read ports onto the identity, network and platform domains the admin
// views use; the legacy repositories satisfy them structurally.
type (
	// OwnerReader resolves a subscription's owning account.
	OwnerReader interface {
		FindOne(ctx context.Context, id int64) (*user.User, error)
	}
	// DeviceReader lists an owner's devices.
	DeviceReader interface {
		QueryDevicePageList(ctx context.Context, userid, subscribeId int64, page, size int) ([]*user.Device, int64, error)
	}
	// TrafficLogReader lists a subscription's traffic records.
	TrafficLogReader interface {
		QueryTrafficLogPageList(ctx context.Context, userId, subscribeId int64, page, size int) ([]*trafficEntity.TrafficLog, int64, error)
	}
	// LogReader filters the audit log.
	LogReader interface {
		FilterSystemLog(ctx context.Context, filter *log.FilterParams) ([]*log.SystemLog, int64, error)
	}
	// CacheInvalidator drops cached account and subscription projections.
	CacheInvalidator interface {
		ClearUserCache(ctx context.Context, data ...*user.User) error
		ClearSubscribeCache(ctx context.Context, data ...*usersub.Subscribe) error
	}
)

type Deps struct {
	Plans    repository.SubscribeRepo
	UserSubs repository.UserSubscriptionRepo
	Users    OwnerReader
	Devices  DeviceReader
	Cache    CacheInvalidator
	Traffic  TrafficLogReader
	Logs     LogReader
	Store    Store
	// SingleModel forbids holding more than one blocking subscription;
	// runtime-mutable, read per request.
	SingleModel func() bool
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) CreateUserSubscribe(ctx context.Context, req *dto.CreateUserSubscribeRequest) error {
	return newCreateUserSubscribeLogic(ctx, s.deps).CreateUserSubscribe(req)
}

func (s *Service) DeleteUserSubscribe(ctx context.Context, req *dto.DeleteUserSubscribeRequest) error {
	return newDeleteUserSubscribeLogic(ctx, s.deps).DeleteUserSubscribe(req)
}

func (s *Service) GetUserSubscribe(ctx context.Context, req *dto.GetUserSubscribeListRequest) (*dto.GetUserSubscribeListResponse, error) {
	return newGetUserSubscribeLogic(ctx, s.deps).GetUserSubscribe(req)
}

func (s *Service) GetUserSubscribeById(ctx context.Context, req *dto.GetUserSubscribeByIdRequest) (*dto.UserSubscribeDetail, error) {
	return newGetUserSubscribeByIdLogic(ctx, s.deps).GetUserSubscribeById(req)
}

func (s *Service) GetUserSubscribeDevices(ctx context.Context, req *dto.GetUserSubscribeDevicesRequest) (*dto.GetUserSubscribeDevicesResponse, error) {
	return newGetUserSubscribeDevicesLogic(ctx, s.deps).GetUserSubscribeDevices(req)
}

func (s *Service) GetUserSubscribeLogs(ctx context.Context, req *dto.GetUserSubscribeLogsRequest) (*dto.GetUserSubscribeLogsResponse, error) {
	return newGetUserSubscribeLogsLogic(ctx, s.deps).GetUserSubscribeLogs(req)
}

func (s *Service) GetUserSubscribeResetTrafficLogs(ctx context.Context, req *dto.GetUserSubscribeResetTrafficLogsRequest) (*dto.GetUserSubscribeResetTrafficLogsResponse, error) {
	return newGetUserSubscribeResetTrafficLogsLogic(ctx, s.deps).GetUserSubscribeResetTrafficLogs(req)
}

func (s *Service) GetUserSubscribeTrafficLogs(ctx context.Context, req *dto.GetUserSubscribeTrafficLogsRequest) (*dto.GetUserSubscribeTrafficLogsResponse, error) {
	return newGetUserSubscribeTrafficLogsLogic(ctx, s.deps).GetUserSubscribeTrafficLogs(req)
}

// Store is the persistence capability required by this package: the
// subscription transaction its read-modify-write operations lock the row in.
type Store interface {
	repository.SubscriptionTransactor
}
