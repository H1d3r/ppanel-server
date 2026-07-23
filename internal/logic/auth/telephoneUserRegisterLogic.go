package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/logic/common"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/log"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/authmethod"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/jwt"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/phone"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/uuidx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type TelephoneUserRegisterLogic struct {
	logger.Logger
	ctx  context.Context
	deps TelephoneUserRegisterDependencies
}

// NewTelephoneUserRegisterLogic User Telephone register
func NewTelephoneUserRegisterLogic(ctx context.Context, deps TelephoneUserRegisterDependencies) *TelephoneUserRegisterLogic {
	return &TelephoneUserRegisterLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *TelephoneUserRegisterLogic) TelephoneUserRegister(req *dto.TelephoneRegisterRequest) (resp *dto.LoginResponse, err error) {
	if err := l.deps.Policy.EnsureRegistrationOpen(l.ctx, authmethod.Mobile); err != nil {
		return nil, err
	}
	if err := l.deps.Policy.VerifyHuman(l.ctx, req.CfToken, req.IP); err != nil {
		return nil, err
	}
	if !phone.Check(req.TelephoneAreaCode, req.Telephone) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "telephone number error")
	}

	phoneNumber, err := phone.FormatToE164(req.TelephoneAreaCode, req.Telephone)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
	}

	// if the email verification is enabled, the verification code is required
	cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeTelephoneCacheKey, constant.ParseVerifyType(uint8(constant.Register)), phoneNumber)
	if err := common.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}
	// Check if the user exists
	_, err = l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, authmethod.Mobile, phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("FindOneByTelephone Error", logger.Field("error", err))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "telephone already exists")
	}
	var referer *user.User
	if req.Invite == "" {
		if l.deps.Config.InviteForced {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.InviteCodeError), "invite code is required")
		}
	} else {
		// Check if the invite code is valid
		referer, err = l.deps.Store.User().FindOneByReferCode(l.ctx, req.Invite)
		if err != nil {
			l.Errorw("FindOneByReferCode Error", logger.Field("error", err))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.InviteCodeError), "invite code is invalid")
		}
	}
	if err := l.deps.Policy.TakeIPPermit(l.ctx, req.IP); err != nil {
		return nil, err
	}
	if err := common.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}

	// Generate password
	pwd := tool.EncodePassWord(req.Password)
	userInfo := &user.User{
		Password:          pwd,
		Algo:              tool.PasswordAlgoArgon2id,
		OnlyFirstPurchase: &l.deps.Config.OnlyFirstPurchase,
		AuthMethods: []user.AuthMethods{
			{
				AuthType:       authmethod.Mobile,
				AuthIdentifier: phoneNumber,
				Verified:       true,
			},
		},
	}
	if referer != nil {
		userInfo.RefererId = referer.Id
	}
	var trialSubscribe *user.Subscribe
	err = l.deps.Store.InTx(l.ctx, func(store repository.Store) error {
		// Save user information
		if err := store.User().Insert(l.ctx, userInfo); err != nil {
			return err
		}
		// Generate ReferCode
		userInfo.ReferCode = uuidx.UserInviteCode(userInfo.Id)
		// Update ReferCode
		if err := store.User().Update(l.ctx, userInfo); err != nil {
			return err
		}
		if l.deps.Config.TrialEnabled {
			// Active trial
			trialSubscribe, err = l.activeTrial(store, userInfo.Id)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	l.clearTrialSubscribeCache(trialSubscribe)

	// Bind device to user if identifier is provided
	if req.Identifier != "" && l.deps.DeviceBinder != nil {
		if err := l.deps.DeviceBinder.BindDeviceToUser(req.Identifier, req.IP, req.UserAgent, userInfo.Id); err != nil {
			l.Errorw("failed to bind device to user",
				logger.Field("user_id", userInfo.Id),
				logger.Field("identifier", req.Identifier),
				logger.Field("error", err.Error()),
			)
			// Don't fail register if device binding fails, just log the error
		}
	}
	if l.ctx.Value(constant.LoginType) != nil {
		req.LoginType = l.ctx.Value(constant.LoginType).(string)
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
		jwt.WithOption("LoginType", req.LoginType),
	)
	if err != nil {
		l.Logger.Error("[UserLogin] token generate error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "token generate error: %v", err.Error())
	}
	sessionIdCacheKey := fmt.Sprintf("%v:%v", config.SessionIdKey, sessionId)
	if err = l.deps.Redis.Set(l.ctx, sessionIdCacheKey, userInfo.Id, time.Duration(l.deps.Config.JWTAccessExpire)*time.Second).Err(); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "set session id error: %v", err.Error())
	}

	defer func() {
		if token != "" && userInfo.Id != 0 {
			loginLog := log.Login{
				Method:    "mobile",
				LoginIP:   req.IP,
				UserAgent: req.UserAgent,
				Success:   token != "",
				Timestamp: timeutil.Now().UnixMilli(),
			}
			content, _ := loginLog.Marshal()
			if err := l.deps.Store.Log().Insert(l.ctx, &log.SystemLog{
				Id:       0,
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

			// Register log
			registerLog := log.Register{
				AuthMethod: "mobile",
				Identifier: phoneNumber,
				RegisterIP: req.IP,
				UserAgent:  req.UserAgent,
				Timestamp:  timeutil.Now().UnixMilli(),
			}
			content, _ = registerLog.Marshal()
			if err := l.deps.Store.Log().Insert(l.ctx, &log.SystemLog{
				Type:     log.TypeRegister.Uint8(),
				ObjectID: userInfo.Id,
				Date:     timeutil.Now().Format("2006-01-02"),
				Content:  string(content),
			}); err != nil {
				l.Errorw("failed to insert login log",
					logger.Field("user_id", userInfo.Id),
					logger.Field("ip", req.IP),
					logger.Field("error", err.Error()))
			}
		}
	}()
	return &dto.LoginResponse{
		Token: token,
	}, nil
}

func (l *TelephoneUserRegisterLogic) activeTrial(store repository.Store, uid int64) (*user.Subscribe, error) {
	sub, err := store.Subscribe().FindOne(l.ctx, l.deps.Config.TrialSubscribeID)
	if err != nil {
		return nil, err
	}
	userSub := &user.Subscribe{
		Id:          0,
		UserId:      uid,
		OrderId:     0,
		SubscribeId: sub.Id,
		StartTime:   timeutil.Now(),
		ExpireTime:  tool.AddTime(l.deps.Config.TrialTimeUnit, l.deps.Config.TrialTime, timeutil.Now()),
		Traffic:     sub.Traffic,
		Download:    0,
		Upload:      0,
		Token:       uuidx.SubscribeToken(fmt.Sprintf("Trial-%v-%s", uid, uuidx.NewUUID().String())),
		UUID:        uuidx.NewUUID().String(),
		Status:      1,
	}
	return userSub, store.UserSubscription().InsertSubscribe(l.ctx, userSub)
}

func (l *TelephoneUserRegisterLogic) clearTrialSubscribeCache(trialSub *user.Subscribe) {
	if trialSub == nil {
		return
	}
	if err := l.deps.Store.UserCache().ClearSubscribeCache(l.ctx, trialSub); err != nil {
		l.Errorw("ClearSubscribeCache failed",
			logger.Field("error", err.Error()),
			logger.Field("user_subscribe_id", trialSub.Id),
		)
	}
	if err := l.deps.Store.Subscribe().ClearCache(l.ctx, trialSub.SubscribeId); err != nil {
		l.Errorw("Clear subscribe cache failed",
			logger.Field("error", err.Error()),
			logger.Field("subscribe_id", trialSub.SubscribeId),
		)
	}
}
