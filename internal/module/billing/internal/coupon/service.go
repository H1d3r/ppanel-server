// Package coupon implements the coupon management subdomain of the billing
// module. Only the module facade may reach it.
package coupon

import (
	"context"
	"crypto/rand"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	entity "github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type Service struct {
	repo repository.CouponRepo
}

func NewService(repo repository.CouponRepo) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req *dto.CreateCouponRequest) error {
	if err := validateCouponInput(req); err != nil {
		return err
	}
	generateCode := req.Code == ""
	couponInfo := &entity.Coupon{}
	mapping.DeepCopy(couponInfo, req)
	couponInfo.Subscribe = slicesx.Int64SliceToString(req.Subscribe)
	if req.Enable == nil {
		enabled := true
		couponInfo.Enable = &enabled
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if generateCode {
			// Coupon codes need unpredictable uniqueness, not a machine/clock
			// based numeric ID. Keep the database's unique constraint authoritative.
			req.Code = random.StrToDashedString(rand.Text())
			couponInfo.Code = req.Code
			couponInfo.Id = 0
		}
		err = s.repo.Insert(ctx, couponInfo)
		if err == nil {
			return nil
		}
		if !generateCode || !errors.Is(err, gorm.ErrDuplicatedKey) {
			break
		}
	}
	return xerr.Wrapf(err, xerr.DatabaseInsertError, "create coupon")
}

func (s *Service) Update(ctx context.Context, req *dto.UpdateCouponRequest) error {
	input := &dto.CreateCouponRequest{
		Name: req.Name, Code: req.Code, Count: req.Count, Type: req.Type,
		Discount: req.Discount, StartTime: req.StartTime, ExpireTime: req.ExpireTime,
		UserLimit: req.UserLimit, Subscribe: req.Subscribe, UsedCount: req.UsedCount, Enable: req.Enable,
	}
	if err := validateCouponInput(input); err != nil {
		return err
	}
	existing, err := s.repo.FindOne(ctx, req.Id)
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find coupon %d", req.Id)
	}
	if req.UsedCount < existing.UsedCount {
		return errors.Wrapf(xerr.NewErrCode(xerr.CouponUsedCountImmutable), "used count cannot be reduced")
	}
	couponInfo := &entity.Coupon{}
	mapping.DeepCopy(couponInfo, req)
	couponInfo.Subscribe = slicesx.Int64SliceToString(req.Subscribe)
	if couponInfo.Enable == nil {
		couponInfo.Enable = existing.Enable
	}
	if err := s.repo.Update(ctx, couponInfo); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update coupon %d", req.Id)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, req *dto.DeleteCouponRequest) error {
	if err := s.repo.Delete(ctx, req.Id); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseDeletedError, "delete coupon %d", req.Id)
	}
	return nil
}

func (s *Service) BatchDelete(ctx context.Context, req *dto.BatchDeleteCouponRequest) error {
	if err := s.repo.BatchDelete(ctx, req.Ids); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseDeletedError, "batch delete coupons")
	}
	return nil
}

func (s *Service) List(ctx context.Context, req *dto.GetCouponListRequest) (*dto.GetCouponListResponse, error) {
	resp := &dto.GetCouponListResponse{}
	total, list, err := s.repo.QueryCouponListByPage(ctx, int(req.Page), int(req.Size), req.Subscribe, req.Search)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "get coupon list")
	}
	resp.Total = total
	resp.List = make([]dto.Coupon, 0)
	for _, item := range list {
		couponInfo := dto.Coupon{}
		mapping.DeepCopy(&couponInfo, item)
		plans, parseErr := slicesx.ParseInt64CSV(item.Subscribe)
		if parseErr != nil {
			return nil, xerr.Wrapf(parseErr, xerr.ERROR, "coupon %d plans: %v", item.Id, parseErr)
		}
		couponInfo.Subscribe = plans
		resp.List = append(resp.List, couponInfo)
	}
	return resp, nil
}

func validateCouponInput(req *dto.CreateCouponRequest) error {
	if req.Count < 0 || req.UsedCount < 0 || req.UserLimit < 0 || req.StartTime <= 0 || req.ExpireTime <= req.StartTime {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidCoupon), "invalid coupon limits or validity window")
	}
	if req.Count > 0 && req.UsedCount > req.Count {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidCoupon), "used count exceeds coupon count")
	}
	switch req.Type {
	case entity.TypePercentage:
		if req.Discount <= 0 || req.Discount > 100 {
			return errors.Wrapf(xerr.NewErrCode(xerr.InvalidCouponDiscount), "percentage discount must be between 1 and 100")
		}
	case entity.TypeFixed:
		if req.Discount <= 0 {
			return errors.Wrapf(xerr.NewErrCode(xerr.InvalidCouponDiscount), "fixed discount must be positive")
		}
	default:
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidCouponType), "unsupported coupon type")
	}
	return nil
}
