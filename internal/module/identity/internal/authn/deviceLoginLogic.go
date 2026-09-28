package authn

import (
	"context"
	"errors"
	"strings"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/account"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// DeviceLogin signs in with a device identifier. A device seen for the first
// time registers a new account when registration is open.
func (s *Service) DeviceLogin(ctx context.Context, req *dto.DeviceLoginRequest) (resp *dto.LoginResponse, err error) {
	if req.Identifier == "" || len(req.Identifier) > 255 || strings.TrimSpace(req.Identifier) != req.Identifier {
		return nil, xerr.NewErrCode(xerr.InvalidParams)
	}
	if err := s.policy.EnsureMethodEnabled(ctx, identifier.Device); err != nil {
		return nil, err
	}
	if s.deps.Config().DeviceOnlyReal {
		if secure, _ := ctx.Value(requestctx.CtxKeyDeviceSecure).(bool); !secure {
			return nil, xerr.Errorf(xerr.InvalidAccess, "verified device transport is required")
		}
	}
	attempt := account.NewAttempt(s.deps.Store.Log(), identifier.Device)
	defer func() {
		if err = attempt.Finish(ctx, err); err != nil {
			resp = nil
		}
	}()

	devices := s.deps.Store.UserDevice()
	deviceInfo, err := devices.FindOneDeviceByIdentifier(ctx, req.Identifier)
	var userInfo *user.User
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if userInfo, deviceInfo, err = s.registerDevice(ctx, req); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find device")
	default:
		if userInfo, err = s.deps.Store.User().FindOne(ctx, deviceInfo.UserId); err != nil {
			return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find user %d of the device", deviceInfo.UserId)
		}
	}
	attempt.Identify(userInfo.Id)
	if err := account.EnsureActive(userInfo); err != nil {
		return nil, err
	}
	// Read authoritative device state rather than trusting a cached binding.
	deviceInfo, err = devices.FindDeviceForAuth(ctx, deviceInfo.Id)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "read device state")
	}
	if !deviceInfo.Enabled || deviceInfo.UserId != userInfo.Id {
		return nil, xerr.Errorf(xerr.InvalidAccess, "device is disabled or its binding changed")
	}
	ip, userAgent := deviceMetadata(ctx)
	touched, err := devices.TouchDevice(ctx, deviceInfo.Id, userInfo.Id, ip, userAgent)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseUpdateError, "refresh device")
	}
	if !touched {
		return nil, xerr.Errorf(xerr.InvalidAccess, "device binding changed")
	}
	token, err := account.IssueSession(ctx, s.deps.Redis, s.deps.Config().sessions(), userInfo.Id, identifier.Device, deviceInfo)
	if err != nil {
		return nil, err
	}
	return &dto.LoginResponse{Token: token}, nil
}

// registerDevice creates the account of a device seen for the first time,
// signed in to by the device alone.
func (s *Service) registerDevice(ctx context.Context, req *dto.DeviceLoginRequest) (*user.User, *user.Device, error) {
	if err := s.policy.EnsureRegistrationOpen(ctx, identifier.Device); err != nil {
		return nil, nil, err
	}
	if err := s.policy.VerifyHuman(ctx, registerpolicy.Register, req.CfToken); err != nil {
		return nil, nil, err
	}
	referer, err := s.resolveReferer(ctx, req.Invite)
	if err != nil {
		return nil, nil, err
	}
	if err := s.policy.TakeIPPermit(ctx); err != nil {
		return nil, nil, err
	}

	cfg := s.deps.Config()
	ip, userAgent := deviceMetadata(ctx)
	newUser := &user.User{OnlyFirstPurchase: &cfg.OnlyFirstPurchase}
	if referer != nil {
		newUser.RefererId = referer.Id
	}
	device := &user.Device{Ip: ip, UserAgent: userAgent, Identifier: req.Identifier, Enabled: true}
	if err := account.Register(ctx, s.deps.Store, account.New{
		User:       newUser,
		Identities: []user.AuthMethods{{AuthType: identifier.Device, AuthIdentifier: req.Identifier, Verified: true}},
		Device:     device,
	}, identifier.Device); err != nil {
		return nil, nil, err
	}
	logger.WithContext(ctx).Infow("device registered a new account",
		logger.Field("user_id", newUser.Id), logger.Field("identifier", req.Identifier))
	return newUser, device, nil
}
