package portal

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/auth/challenge"
	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/internal/checkout"
	"github.com/perfect-panel/server/internal/module/billing/internal/gateway"
	"github.com/perfect-panel/server/internal/module/billing/internal/orderaudit"
	"github.com/perfect-panel/server/internal/module/billing/internal/ordercontext"
	"github.com/perfect-panel/server/internal/module/billing/internal/pricing"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// maxPendingGuestOrders caps the unpaid guest orders one identity may hold.
// Each pending order reserves plan inventory and a coupon use until it
// closes, and only one of them can ever create the identity's account.
const maxPendingGuestOrders = 3

// NormalizeGuestIdentity validates the account a guest purchase creates and
// returns its canonical auth type and identifier. Only email and mobile are
// accepted: the paid order inserts the auth method as given and issues a
// session, so an OAuth or device identifier would let a buyer pre-bind
// someone else's third-party identity and take over its first sign-in.
// Mobile numbers must carry their country calling code; they are stored in
// E.164 like the telephone registration and login flows store them.
func NormalizeGuestIdentity(authType, value string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case identifier.Email:
		email, err := identifier.ValidateEmail(value, "", false)
		if err != nil {
			return "", "", xerr.Wrapf(err, xerr.InvalidParams, "invalid guest email")
		}
		return identifier.Email, email, nil
	case identifier.Mobile:
		number := strings.TrimPrefix(strings.TrimSpace(value), "+")
		if number == "" || !identifier.CheckPhone(number) {
			return "", "", xerr.Errorf(xerr.InvalidParams, "invalid guest mobile number")
		}
		e164, err := identifier.FormatToE164("", number)
		if err != nil {
			return "", "", xerr.Wrapf(err, xerr.InvalidParams, "invalid guest mobile number")
		}
		return identifier.Mobile, e164, nil
	default:
		return "", "", xerr.Errorf(xerr.InvalidParams, "unsupported guest auth type")
	}
}

// verifyGuestHuman applies the registration Turnstile check to a guest
// purchase before it touches any account or reservation.
func (s *Service) verifyGuestHuman(ctx context.Context, token string) error {
	if s.deps.Config.GuestVerification == nil {
		return nil
	}
	policy := s.deps.Config.GuestVerification()
	if !policy.Enabled {
		return nil
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(policy.Secret) == "" {
		return xerr.Errorf(xerr.TooManyRequests, "guest purchase verification failed")
	}
	verify := s.deps.VerifyTurnstile
	if verify == nil {
		verify = verifyTurnstile
	}
	metadata, _ := requestmeta.From(ctx)
	ok, err := verify(ctx, policy.Secret, token, metadata.ClientIP)
	if err != nil {
		return xerr.Wrapf(err, xerr.TooManyRequests, "guest purchase verification failed")
	}
	if !ok {
		return xerr.Errorf(xerr.TooManyRequests, "guest purchase verification failed")
	}
	return nil
}

func verifyTurnstile(ctx context.Context, secret, token, remoteIP string) (bool, error) {
	return challenge.New(challenge.Config{Secret: secret, Timeout: 3 * time.Second}).Verify(ctx, token, remoteIP)
}

// Purchase creates a guest pre-order: the billing transaction reserves the
// coupon and creates the pending order, then plan inventory is reserved in
// its own subscription-domain transaction (ADR-001 step 2). A guest holds no
// gift credit, so the order is priced exactly like PrePurchase previews it.
func (s *Service) Purchase(ctx context.Context, req *dto.PortalPurchaseRequest) (*dto.PortalPurchaseResponse, error) {
	authType, guestIdentifier, err := NormalizeGuestIdentity(req.AuthType, req.Identifier)
	if err != nil {
		return nil, err
	}
	if err := s.verifyGuestHuman(ctx, req.TurnstileToken); err != nil {
		return nil, err
	}
	userAuth, err := s.deps.UserAuths.FindUserAuthMethodByOpenID(ctx, authType, guestIdentifier)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find user auth")
	}
	if userAuth != nil && userAuth.UserId != 0 {
		return nil, xerr.Errorf(xerr.UserExist, "user already exists")
	}
	// The cap is best effort under concurrent requests for one identity;
	// the Turnstile check is the rate control.
	pending, err := s.deps.Orders.CountPendingGuestOrders(ctx, authType, guestIdentifier)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "count pending guest orders")
	}
	if pending >= maxPendingGuestOrders {
		return nil, xerr.Errorf(xerr.TooManyRequests, "too many pending guest orders")
	}
	plan, err := s.deps.Plans.FindOne(ctx, req.SubscribeId)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find subscribe %d", req.SubscribeId)
	}
	if plan.Inventory == 0 {
		return nil, xerr.Errorf(xerr.SubscribeOutOfStock, "subscribe out of stock")
	}
	if plan.Sell == nil || !*plan.Sell {
		return nil, xerr.Errorf(xerr.ERROR, "subscribe not sell")
	}
	terms, err := checkout.ResolvePlanTerms(ctx, s.deps.Coupons, s.deps.Payments, plan, req.Quantity, req.Coupon, req.Payment)
	if err != nil {
		return nil, err
	}
	if terms.Method == nil {
		return nil, xerr.Errorf(xerr.PaymentMethodNotFound, "payment method is required")
	}
	if gateway.IsBalance(terms.Method) {
		return nil, xerr.Errorf(xerr.PaymentMethodNotFound, "balance error")
	}
	checkoutToken := ordercontext.GuestCheckoutToken(ctx)
	if checkoutToken == "" {
		checkoutToken = random.KeyNew(32, 1)
	}
	orderInfo := &order.Order{
		OrderNo:                order.GenerateTradeNo(),
		Type:                   order.TypeSubscribe,
		Quantity:               req.Quantity,
		Coupon:                 req.Coupon,
		PaymentId:              terms.Method.Id,
		Method:                 terms.Method.Platform,
		Status:                 order.StatusPending,
		IsNew:                  true,
		SubscribeId:            req.SubscribeId,
		GuestAuthType:          authType,
		GuestIdentifier:        guestIdentifier,
		GuestPasswordHash:      password.EncodePassWord(req.Password),
		GuestInviteCode:        req.InviteCode,
		GuestCheckoutTokenHash: order.CheckoutTokenHash(checkoutToken),
	}
	checkout.ApplyQuote(orderInfo, pricing.Compute(terms.Input(0)))
	ordercontext.ApplyIdempotency(ctx, orderInfo)
	// Billing-domain transaction: coupon reservation and order creation
	// settle together.
	err = s.deps.Tx.InBillingTx(ctx, func(tx repository.BillingStore) error {
		if err := checkout.ReserveCoupon(ctx, tx, orderInfo); err != nil {
			return err
		}
		return checkout.InsertOrder(ctx, tx, orderInfo, orderaudit.SourceGuest)
	})
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.ERROR, "create guest order")
	}
	// Reserve plan inventory in its own subscription-domain transaction
	// (ADR-001 step 2). On failure the guest order is closed inline; guest
	// pre-orders only hold a coupon reservation.
	if err := s.deps.Inventory.Reserve(ctx, orderInfo.OrderNo, plan.Id); err != nil {
		s.closeUnreservedOrder(ctx, orderInfo)
		if errors.Is(err, subscription.ErrOutOfStock) {
			return nil, xerr.Errorf(xerr.SubscribeOutOfStock, "subscribe out of stock")
		}
		return nil, xerr.Wrapf(err, xerr.ERROR, "reserve inventory")
	}
	if err := s.deps.Queue.EnqueueDeferredClose(ctx, orderInfo.OrderNo); err != nil {
		logger.WithContext(ctx).Errorw("[CloseOrder Task] Enqueue task error", logger.Field("error", err.Error()), logger.Field("orderNo", orderInfo.OrderNo))
	} else {
		logger.WithContext(ctx).Infow("[CloseOrder Task] Enqueue task success", logger.Field("orderNo", orderInfo.OrderNo))
	}
	return &dto.PortalPurchaseResponse{OrderNo: orderInfo.OrderNo, CheckoutToken: checkoutToken}, nil
}

// closeUnreservedOrder closes a guest order whose inventory could not be
// reserved and releases its coupon use.
func (s *Service) closeUnreservedOrder(ctx context.Context, orderInfo *order.Order) {
	err := s.deps.Tx.InBillingTx(ctx, func(tx repository.BillingStore) error {
		closed, err := tx.Order().UpdateOrderStatusFrom(ctx, orderInfo.OrderNo, order.StatusPending, order.StatusClosed)
		if err != nil {
			return err
		}
		if closed && orderInfo.CouponReserved {
			return tx.Coupon().ReleaseUsage(ctx, orderInfo.Coupon)
		}
		return nil
	})
	if err != nil {
		logger.WithContext(ctx).Errorw("[Purchase] Close order after reservation failure failed", logger.Field("error", err.Error()), logger.Field("orderNo", orderInfo.OrderNo))
	}
}
