package auth

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/jwt"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/uuidx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type DeviceLoginLogic struct {
	logger.Logger
	ctx  context.Context
	deps DeviceLoginDependencies
}

const deviceRegistrationMethod = "device"

// Device Login
func NewDeviceLoginLogic(ctx context.Context, deps DeviceLoginDependencies) *DeviceLoginLogic {
	return &DeviceLoginLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *DeviceLoginLogic) DeviceLogin(req *dto.DeviceLoginRequest) (resp *dto.LoginResponse, err error) {
	if !l.deps.Config.Enabled {
		return nil, xerr.NewErrMsg("Device login is disabled")
	}
	if l.deps.Config.OnlyRealDevice {
		secure, _ := l.ctx.Value(constant.CtxKeyDeviceSecure).(bool)
		if !secure {
			return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "verified device transport is required")
		}
	}

	loginStatus := false
	var userInfo *user.User
	// Record login status
	defer func() {
		if userInfo != nil && userInfo.Id != 0 {
			loginLog := log.Login{
				Method:    "device",
				LoginIP:   req.IP,
				UserAgent: req.UserAgent,
				Success:   loginStatus,
				Timestamp: timeutil.Now().UnixMilli(),
			}
			content, _ := loginLog.Marshal()
			if err := l.deps.Store.Log().Insert(l.ctx, &log.SystemLog{
				Type:     log.TypeLogin.Uint8(),
				Date:     timeutil.Now().Format("2006-01-02"),
				ObjectID: userInfo.Id,
				Content:  string(content),
			}); err != nil {
				l.Errorw("failed to insert login log",
					logger.Field("user_id", userInfo.Id),
					logger.Field("ip", req.IP),
					logger.Field("error", err.Error()),
				)
			}
		}
	}()

	// Check if device exists by identifier
	deviceInfo, err := l.deps.Store.UserDevice().FindOneDeviceByIdentifier(l.ctx, req.Identifier)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Device not found, create new user and device
			userInfo, err = l.registerUserAndDevice(req)
			if err != nil {
				return nil, err
			}
		} else {
			l.Errorw("query device failed",
				logger.Field("identifier", req.Identifier),
				logger.Field("error", err.Error()),
			)
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query device failed: %v", err.Error())
		}
	} else {
		// Device found, get user info
		userInfo, err = l.deps.Store.User().FindOne(l.ctx, deviceInfo.UserId)
		if err != nil {
			l.Errorw("query user failed",
				logger.Field("user_id", deviceInfo.UserId),
				logger.Field("error", err.Error()),
			)
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user failed: %v", err.Error())
		}
	}

	// Generate session id
	sessionId := uuidx.NewUUID().String()

	// Generate token
	token, err := jwt.NewJwtToken(
		l.deps.Config.JWTAccessSecret,
		timeutil.Now().Unix(),
		l.deps.Config.JWTAccessExpire,
		jwt.WithOption("UserId", userInfo.Id),
		jwt.WithOption("SessionId", sessionId),
		jwt.WithOption("LoginType", "device"),
	)
	if err != nil {
		l.Errorw("token generate error",
			logger.Field("user_id", userInfo.Id),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "token generate error: %v", err.Error())
	}

	// Store session id in redis
	sessionIdCacheKey := fmt.Sprintf("%v:%v", config.SessionIdKey, sessionId)
	if err = l.deps.Redis.Set(l.ctx, sessionIdCacheKey, userInfo.Id, time.Duration(l.deps.Config.JWTAccessExpire)*time.Second).Err(); err != nil {
		l.Errorw("set session id error",
			logger.Field("user_id", userInfo.Id),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "set session id error: %v", err.Error())
	}

	loginStatus = true
	return &dto.LoginResponse{
		Token: token,
	}, nil
}

func (l *DeviceLoginLogic) registerUserAndDevice(req *dto.DeviceLoginRequest) (*user.User, error) {
	l.Infow("device not found, creating new user and device",
		logger.Field("identifier", req.Identifier),
		logger.Field("ip", req.IP),
	)

	if err := l.deps.Policy.EnsureRegistrationOpen(l.ctx, deviceRegistrationMethod); err != nil {
		return nil, err
	}
	if err := l.deps.Policy.VerifyHuman(l.ctx, req.CfToken, req.IP); err != nil {
		return nil, err
	}
	var referer *user.User
	if req.Invite == "" {
		if l.deps.Config.InviteForced {
			return nil, errors.Wrap(xerr.NewErrCode(xerr.InviteCodeError), "invite code is required")
		}
	} else {
		var err error
		referer, err = l.deps.Store.User().FindOneByReferCode(l.ctx, req.Invite)
		if err != nil {
			return nil, errors.Wrap(xerr.NewErrCode(xerr.InviteCodeError), "invite code is invalid")
		}
	}
	if err := l.deps.Policy.TakeIPPermit(l.ctx, req.IP); err != nil {
		return nil, err
	}

	var userInfo *user.User
	err := l.deps.Store.InIdentityTx(l.ctx, func(store repository.IdentityStore) error {
		// Create new user
		userInfo = &user.User{
			OnlyFirstPurchase: &l.deps.Config.OnlyFirstPurchase,
		}
		if referer != nil {
			userInfo.RefererId = referer.Id
		}
		if err := store.User().Insert(l.ctx, userInfo); err != nil {
			l.Errorw("failed to create user",
				logger.Field("error", err.Error()),
			)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create user failed: %v", err)
		}

		// Update refer code
		userInfo.ReferCode = uuidx.UserInviteCode(userInfo.Id)
		if err := store.User().Update(l.ctx, userInfo); err != nil {
			l.Errorw("failed to update refer code",
				logger.Field("user_id", userInfo.Id),
				logger.Field("error", err.Error()),
			)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update refer code failed: %v", err)
		}

		// Create device auth method
		authMethod := &user.AuthMethods{
			UserId:         userInfo.Id,
			AuthType:       "device",
			AuthIdentifier: req.Identifier,
			Verified:       true,
		}
		if err := store.UserAuth().InsertUserAuthMethods(l.ctx, authMethod); err != nil {
			l.Errorw("failed to create device auth method",
				logger.Field("user_id", userInfo.Id),
				logger.Field("identifier", req.Identifier),
				logger.Field("error", err.Error()),
			)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create device auth method failed: %v", err)
		}

		// Insert device record
		deviceInfo := &user.Device{
			Ip:         req.IP,
			UserId:     userInfo.Id,
			UserAgent:  req.UserAgent,
			Identifier: req.Identifier,
			Enabled:    true,
			Online:     false,
		}
		if err := store.UserDevice().InsertDevice(l.ctx, deviceInfo); err != nil {
			l.Errorw("failed to insert device",
				logger.Field("user_id", userInfo.Id),
				logger.Field("identifier", req.Identifier),
				logger.Field("error", err.Error()),
			)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert device failed: %v", err)
		}

		// Registration emits the domain event; the subscription module
		// grants the trial when it consumes it (idempotent, retried by
		// the dispatcher).
		if err := store.Outbox().Append(l.ctx, "identity.user_registered", strconv.FormatInt(userInfo.Id, 10), "{}"); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		l.Errorw("device registration failed",
			logger.Field("identifier", req.Identifier),
			logger.Field("error", err.Error()),
		)
		return nil, err
	}

	l.Infow("device registration completed successfully",
		logger.Field("user_id", userInfo.Id),
		logger.Field("identifier", req.Identifier),
		logger.Field("refer_code", userInfo.ReferCode),
	)

	// Register log
	registerLog := log.Register{
		AuthMethod: "device",
		Identifier: req.Identifier,
		RegisterIP: req.IP,
		UserAgent:  req.UserAgent,
		Timestamp:  timeutil.Now().UnixMilli(),
	}
	content, _ := registerLog.Marshal()

	if err := l.deps.Store.Log().Insert(l.ctx, &log.SystemLog{
		Type:     log.TypeRegister.Uint8(),
		Date:     timeutil.Now().Format("2006-01-02"),
		ObjectID: userInfo.Id,
		Content:  string(content),
	}); err != nil {
		l.Errorw("failed to insert register log",
			logger.Field("user_id", userInfo.Id),
			logger.Field("ip", req.IP),
			logger.Field("error", err.Error()),
		)
	}

	return userInfo, nil
}
