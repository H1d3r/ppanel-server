package fulfillment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/entitlement"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"uuid"
)

var ErrEntitlementConflict = errors.New("entitlement ownership or revision conflict")

func entitlementKey(parts ...string) string {
	data, _ := json.Marshal(parts)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ReconcileEntitlement projects a verified, reconciled billing snapshot. It
// deliberately has no HTTP adapter and performs no Apple signature validation.
// The billing adapter must supply a durable revision after resolving historical
// transactions and revocations; raw notification delivery order is not valid.
func (s *Service) ReconcileEntitlement(ctx context.Context, cmd dto.ReconcileEntitlementCommand) (*dto.EntitlementResult, error) {
	if err := validateEntitlement(cmd); err != nil {
		return nil, err
	}
	cmd.PeriodStart = cmd.PeriodStart.UTC().Truncate(time.Millisecond)
	cmd.PeriodEnd = cmd.PeriodEnd.UTC().Truncate(time.Millisecond)
	cmd.GraceUntil = canonicalTime(cmd.GraceUntil)
	cmd.RevokedAt = canonicalTime(cmd.RevokedAt)
	if err := validateEntitlement(cmd); err != nil {
		return nil, err
	}
	orderInfo, err := s.deps.Orders.FindOne(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if orderInfo.UserId != cmd.UserID || orderInfo.SubscribeId != cmd.PlanID || orderInfo.TradeNo != cmd.TransactionKey || orderInfo.Method != "AppleIAP" || (orderInfo.Status != 2 && orderInfo.Status != 5) {
		return nil, ErrEntitlementConflict
	}
	return s.reconcileEntitlement(ctx, cmd)
}

func canonicalTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC().Truncate(time.Millisecond)
	return &v
}

func (s *Service) reconcileEntitlement(ctx context.Context, cmd dto.ReconcileEntitlementCommand) (*dto.EntitlementResult, error) {
	stateID := entitlementKey(cmd.Source, cmd.Scope, cmd.SubscriptionKey)
	periodID := entitlementKey(cmd.Source, cmd.Scope, cmd.TransactionKey)
	payload, _ := json.Marshal(cmd)
	fingerprint := entitlementKey(string(payload))
	result := &dto.EntitlementResult{}
	var projected *usersub.Subscribe
	err := s.deps.Store.InSubscriptionTx(ctx, func(store repository.SubscriptionStore) error {
		// The user lock serializes first purchases and quota checks. The state
		// primary key additionally protects the same chain across users.
		if err := store.UserSubscription().LockUserSerial(ctx, cmd.UserID); err != nil {
			return err
		}
		state, err := store.Entitlement().FindStateForUpdate(ctx, stateID)
		if err != nil {
			return err
		}
		if state != nil {
			if state.UserID != cmd.UserID || state.Mode != cmd.Mode {
				return ErrEntitlementConflict
			}
			projected, err = store.UserSubscription().FindOneSubscribeForUpdate(ctx, state.UserSubscribeID)
			if err != nil {
				return err
			}
			if projected.UserId != cmd.UserID || projected.EntitlementSource != cmd.Source {
				return ErrEntitlementConflict
			}
			if cmd.Revision <= state.Revision {
				if cmd.Revision == state.Revision && fingerprint != state.Fingerprint {
					return ErrEntitlementConflict
				}
				result.UserSubscribeID, result.Revision, result.AccessUntil = projected.Id, state.Revision, projected.ExpireTime
				return nil
			}
		}
		plan, err := store.Subscribe().FindOne(ctx, cmd.PlanID)
		if err != nil {
			return err
		}
		period, err := store.Entitlement().FindPeriod(ctx, periodID)
		if err != nil {
			return err
		}
		if period != nil && (period.EntitlementID != stateID || period.PlanID != cmd.PlanID || period.OrderID != cmd.OrderID || period.BillingInterval != cmd.BillingInterval || !period.StartAt.Equal(cmd.PeriodStart)) {
			return ErrEntitlementConflict
		}
		trafficLimit := plan.Traffic
		if period != nil {
			trafficLimit = period.TrafficLimit
		}
		now := timeutil.Now()
		until := entitlementAccessUntil(cmd)
		if state == nil {
			if s.deps.SingleModel != nil && s.deps.SingleModel() {
				blocked, err := store.UserSubscription().HasBlockingSubscription(ctx, cmd.UserID)
				if err != nil {
					return err
				}
				if blocked {
					return fmt.Errorf("single subscription mode exceeds limit; entitlement delivery must be retried")
				}
			}
			if plan.Quota > 0 {
				count, err := store.UserSubscription().CountQuotaConsumingSubscriptions(ctx, cmd.UserID, cmd.PlanID)
				if err != nil {
					return err
				}
				if count >= plan.Quota {
					return fmt.Errorf("subscription quota exceeded; entitlement delivery must be retried")
				}
			}
			projected = &usersub.Subscribe{UserId: cmd.UserID, OrderId: cmd.OrderID, SubscribeId: cmd.PlanID,
				StartTime: cmd.PeriodStart, ExpireTime: until, Traffic: trafficLimit,
				Token: usersub.TokenFromOrder("entitlement:" + stateID), UUID: uuid.NewV4().String(), EntitlementSource: cmd.Source}
			projectEntitlement(projected, cmd, until, now, false)
			if err := store.UserSubscription().InsertSubscribe(ctx, projected); err != nil {
				return err
			}
		} else {
			// Switching plan or refunding does not grant another quota reset.
			// A reset requires a previously unseen provider transaction.
			reset := period == nil && cmd.ResetTraffic && plan.ResetCycle == 0 && !cmd.PeriodStart.After(now) && cmd.PeriodEnd.After(now) && cmd.Status == "active"
			projected.SubscribeId = cmd.PlanID
			projected.Traffic = trafficLimit
			projectEntitlement(projected, cmd, until, now, reset)
			if err := store.UserSubscription().ApplyEntitlementProjection(ctx, projected); err != nil {
				return err
			}
		}
		if period == nil {
			period = &entitlement.Period{ID: periodID, EntitlementID: stateID, TransactionKey: cmd.TransactionKey,
				BillingInterval: cmd.BillingInterval,
				TrafficLimit:    trafficLimit,
				UserSubscribeID: projected.Id, OrderID: cmd.OrderID, PlanID: cmd.PlanID, StartAt: cmd.PeriodStart,
				EndAt: cmd.PeriodEnd, TrafficReset: cmd.ResetTraffic && plan.ResetCycle == 0 && cmd.Status == "active" && !cmd.PeriodStart.After(now) && cmd.PeriodEnd.After(now)}
			if err := store.Entitlement().InsertPeriod(ctx, period); err != nil {
				return err
			}
		} else {
			// End dates may be corrected/extended by the provider. Replaying
			// this period never repeats its original traffic reset.
			period.EndAt = cmd.PeriodEnd
			if err := store.Entitlement().UpdatePeriod(ctx, period); err != nil {
				return err
			}
		}
		newState := &entitlement.State{ID: stateID, Source: cmd.Source, Scope: cmd.Scope, SubscriptionKey: cmd.SubscriptionKey,
			UserID: cmd.UserID, UserSubscribeID: projected.Id, Revision: cmd.Revision, Fingerprint: fingerprint,
			PeriodID: periodID, Mode: cmd.Mode, Status: cmd.Status, AutoRenew: cmd.AutoRenew,
			GraceUntil: cmd.GraceUntil, RevokedAt: cmd.RevokedAt}
		if state == nil {
			err = store.Entitlement().InsertState(ctx, newState)
		} else {
			err = store.Entitlement().UpdateState(ctx, newState)
		}
		if err != nil {
			return err
		}
		if err := store.Entitlement().InsertRevision(ctx, &entitlement.Revision{ID: stateID + ":" + strconv.FormatInt(cmd.Revision, 10), EntitlementID: stateID, Payload: string(payload)}); err != nil {
			return err
		}
		result.UserSubscribeID, result.Revision, result.AccessUntil, result.Applied = projected.Id, cmd.Revision, until, true
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Return cache failures so durable callers retry even after DB commit.
	// Replays perform these invalidations again without mutating periods.
	if err := s.deps.Cache.ClearSubscribeCache(ctx, projected); err != nil {
		return nil, err
	}
	// Include every previously referenced plan: several reconciliations may
	// commit while caches are unavailable, before any retry can invalidate.
	planIDs, err := s.deps.Store.Entitlement().PlanIDs(ctx, stateID)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Plans.ClearCache(ctx, planIDs...); err != nil {
		return nil, err
	}
	return result, nil
}

func entitlementAccessUntil(cmd dto.ReconcileEntitlementCommand) time.Time {
	until := cmd.PeriodEnd
	if cmd.Status == "grace" && cmd.GraceUntil != nil {
		until = *cmd.GraceUntil
	}
	if cmd.RevokedAt != nil && cmd.RevokedAt.Before(until) {
		until = *cmd.RevokedAt
	}
	return until
}

func projectEntitlement(sub *usersub.Subscribe, cmd dto.ReconcileEntitlementCommand, until, now time.Time, reset bool) {
	sub.ExpireTime = until
	if reset {
		sub.Upload, sub.Download = 0, 0
	}
	// Administrative holds and local cancellation are independent of payment.
	if sub.Status == usersub.SubscribeStatusStopped || sub.Status == usersub.SubscribeStatusDeducted {
		return
	}
	if cmd.Status == "revoked" || cmd.Status == "expired" || cmd.Status == "billing_retry" || !until.After(now) || cmd.PeriodStart.After(now) {
		sub.Status = usersub.SubscribeStatusExpired
		sub.FinishedAt = &now
	} else if sub.Traffic > 0 && (sub.Upload >= sub.Traffic || sub.Download >= sub.Traffic-sub.Upload) {
		sub.Status = usersub.SubscribeStatusFinished
		sub.FinishedAt = &now
	} else {
		sub.Status = usersub.SubscribeStatusActive
		sub.FinishedAt = nil
	}
}

func validateEntitlement(c dto.ReconcileEntitlementCommand) error {
	// A future scheduled product is not the current entitlement. The billing
	// scheduler must retain it and issue a new snapshot when it takes effect.
	now := timeutil.Now()
	if c.PeriodStart.After(now) {
		return fmt.Errorf("entitlement period has not started; retry at period start")
	}
	if (c.Status == "expired" || c.Status == "billing_retry") && c.PeriodEnd.After(now) {
		return fmt.Errorf("inactive entitlement has a future paid deadline")
	}
	if c.RevokedAt != nil && c.RevokedAt.After(now) {
		return fmt.Errorf("revocation has not taken effect")
	}
	for _, s := range []string{c.Scope, c.SubscriptionKey, c.TransactionKey} {
		if s == "" || len(s) > 255 || strings.TrimSpace(s) != s {
			return fmt.Errorf("invalid entitlement identity")
		}
	}
	if c.Source != "apple" || c.UserID <= 0 || c.PlanID <= 0 || c.OrderID <= 0 || c.Revision <= 0 || c.PeriodStart.UnixMilli() <= 0 || !c.PeriodEnd.After(c.PeriodStart) {
		return fmt.Errorf("invalid entitlement period")
	}
	if c.Mode != "auto_renewable" {
		return fmt.Errorf("only auto-renewable Apple subscriptions are supported")
	}
	if c.BillingInterval != "month" && c.BillingInterval != "year" {
		return fmt.Errorf("Apple subscriptions must have a monthly or yearly billing interval")
	}
	switch c.Status {
	case "active", "expired", "billing_retry", "revoked", "grace":
	default:
		return fmt.Errorf("invalid entitlement status")
	}
	if c.Status == "grace" && (c.GraceUntil == nil || !c.GraceUntil.After(c.PeriodEnd)) {
		return fmt.Errorf("invalid grace period")
	}
	if c.Status != "grace" && c.GraceUntil != nil {
		return fmt.Errorf("grace deadline without grace state")
	}
	if (c.Status == "revoked") != (c.RevokedAt != nil) || (c.RevokedAt != nil && c.RevokedAt.UnixMilli() <= 0) {
		return fmt.Errorf("invalid revocation")
	}
	return nil
}
