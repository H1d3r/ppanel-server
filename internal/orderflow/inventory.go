package orderflow

import (
	"context"
	"errors"
	"strconv"

	"github.com/perfect-panel/server/internal/repository"
)

// Inbox consumers for the plan-inventory lifecycle (ADR-001 step 2).
// Reservation and restoration are subscription-domain writes that used to sit
// inside billing transactions; the idempotent inbox keys them by order number
// so the purchase flow, the activation safety net and the close-order
// compensation can each run at-least-once without double-moving stock.
const (
	InventoryReserveConsumer = "subscription.inventory_reserve"
	InventoryRestoreConsumer = "subscription.inventory_restore"
)

// ErrOutOfStock reports that the plan has no inventory left to reserve.
var ErrOutOfStock = errors.New("subscribe out of stock")

// ReserveInventoryOnce reserves one unit of plan inventory for the order in a
// subscription-domain transaction. Replays and concurrent deliveries are
// resolved by the inbox marker: a lost race rolls the reservation back and the
// retry sees the winner's marker. Returns ErrOutOfStock when no unit is left.
func ReserveInventoryOnce(ctx context.Context, store repository.Store, orderNo string, subscribeID int64) error {
	mark, err := store.Inbox().Find(ctx, InventoryReserveConsumer, orderNo)
	if err != nil {
		return err
	}
	if mark != nil {
		return nil
	}
	return store.InTx(ctx, func(tx repository.Store) error {
		reserved, err := tx.Subscribe().ReserveInventory(ctx, subscribeID)
		if err != nil {
			return err
		}
		if !reserved {
			return ErrOutOfStock
		}
		return tx.Inbox().Insert(ctx, InventoryReserveConsumer, orderNo, strconv.FormatInt(subscribeID, 10))
	})
}

// RestoreInventoryOnce returns the order's reserved unit exactly once. Orders
// that never reserved (stock-out compensation, historical orders) are a
// no-op, and a second restoration attempt is absorbed by the restore marker.
func RestoreInventoryOnce(ctx context.Context, store repository.Store, orderNo string, subscribeID int64) error {
	reserveMark, err := store.Inbox().Find(ctx, InventoryReserveConsumer, orderNo)
	if err != nil {
		return err
	}
	if reserveMark == nil {
		return nil
	}
	restoreMark, err := store.Inbox().Find(ctx, InventoryRestoreConsumer, orderNo)
	if err != nil {
		return err
	}
	if restoreMark != nil {
		return nil
	}
	return store.InTx(ctx, func(tx repository.Store) error {
		if err := tx.Subscribe().RestoreInventory(ctx, subscribeID); err != nil {
			return err
		}
		return tx.Inbox().Insert(ctx, InventoryRestoreConsumer, orderNo, "")
	})
}
