package payment

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/payment"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdatePaymentMethodLogic struct {
	logger.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdatePaymentMethodLogic Update Payment Method
func NewUpdatePaymentMethodLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePaymentMethodLogic {
	return &UpdatePaymentMethodLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdatePaymentMethodLogic) UpdatePaymentMethod(req *dto.UpdatePaymentMethodRequest) (resp *dto.PaymentConfig, err error) {
	if payment.ParsePlatform(req.Platform) == payment.UNSUPPORTED {
		l.Errorw("unsupported payment platform", logger.Field("mark", req.Platform))
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "UNSUPPORTED_PAYMENT_PLATFORM"), "unsupported payment platform: %s", req.Platform)
	}
	paymentStore := l.svcCtx.Store.Payment()
	method, err := paymentStore.FindOne(l.ctx, req.Id)
	if err != nil {
		l.Errorw("find payment method error", logger.Field("id", req.Id), logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find payment method error: %s", err.Error())
	}
	if method.Platform != req.Platform {
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "PAYMENT_PLATFORM_IMMUTABLE"), "payment platform cannot be changed")
	}
	if err := validatePaymentFee(req.FeeMode, req.FeePercent, req.FeeAmount); err != nil {
		return nil, err
	}
	if req.Sort == 0 {
		req.Sort = method.Sort
	}
	config := parsePaymentPlatformConfig(l.ctx, payment.ParsePlatform(req.Platform), req.Config)
	if config == "" {
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_PAYMENT_CONFIG"), "invalid payment config")
	}
	if method.Config != config || method.Domain != req.Domain {
		pending, err := l.svcCtx.Store.Order().CountPendingByPaymentID(l.ctx, method.Id)
		if err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "count pending payment orders: %v", err)
		}
		if pending > 0 {
			return nil, errors.Wrapf(xerr.NewErrCodeMsg(409, "PAYMENT_METHOD_HAS_PENDING_ORDERS"), "payment method has %d pending orders", pending)
		}
	}
	tool.DeepCopy(method, req)
	method.Config = config
	if err := paymentStore.Update(l.ctx, method); err != nil {
		l.Errorw("update payment method error", logger.Field("id", req.Id), logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update payment method error: %s", err.Error())
	}
	resp = &dto.PaymentConfig{}
	tool.DeepCopy(resp, method)
	var configMap map[string]interface{}
	_ = json.Unmarshal([]byte(method.Config), &configMap)
	resp.Config = configMap
	return
}
