// Package deduction provides functionality for calculating remaining amounts
// in subscription billing systems, supporting various time units and traffic-based calculations.
package deduction

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/perfect-panel/server/pkg/timeutil"
)

const (
	// Time unit constants for subscription billing
	UnitTimeNoLimit = "NoLimit" // Unlimited time subscription
	UnitTimeYear    = "Year"    // Annual subscription
	UnitTimeMonth   = "Month"   // Monthly subscription
	UnitTimeDay     = "Day"     // Daily subscription
	UnitTimeHour    = "Hour"    // Hourly subscription
	UnitTimeMinute  = "Minute"  // Per-minute subscription

	// Reset cycle constants for traffic resets
	ResetCycleNone    = 0 // No reset cycle
	ResetCycle1st     = 1 // Reset on 1st of each month
	ResetCycleMonthly = 2 // Reset monthly based on start date
	ResetCycleYear    = 3 // Reset yearly based on start date
)

// Error definitions for validation and calculation failures
var (
	ErrInvalidAmount         = errors.New("order amount cannot be negative")
	ErrInvalidTraffic        = errors.New("traffic values cannot be negative")
	ErrInvalidTimeRange      = errors.New("expire time must be after start time")
	ErrInvalidUnitTime       = errors.New("invalid unit time")
	ErrInvalidDeductionRatio = errors.New("deduction ratio must be between 0 and 100")
)

// Subscribe represents a subscription with time and traffic limits
type Subscribe struct {
	StartTime      time.Time // Subscription start time
	ExpireTime     time.Time // Subscription expiration time
	Traffic        int64     // Traffic allowance per reset cycle in bytes (0 = unlimited)
	Download       int64     // Downloaded traffic in the current cycle in bytes
	Upload         int64     // Uploaded traffic in the current cycle in bytes
	UnitTime       string    // Time unit for billing (Year, Month, Day, etc.)
	ResetCycle     int64     // Traffic reset cycle
	DeductionRatio int64     // Deduction ratio for weighted calculations (0-100)
}

// Order represents what was paid for the subscription term
type Order struct {
	Amount int64 // Total amount paid for the term from StartTime to ExpireTime
}

// Validate checks if the Subscribe struct contains valid data
func (s *Subscribe) Validate() error {
	if s.Traffic < 0 || s.Download < 0 || s.Upload < 0 {
		return ErrInvalidTraffic
	}

	if s.Download+s.Upload > s.Traffic {
		return fmt.Errorf("download + upload (%d) cannot exceed total traffic (%d)", s.Download+s.Upload, s.Traffic)
	}

	if !s.ExpireTime.After(s.StartTime) {
		return ErrInvalidTimeRange
	}

	if s.DeductionRatio < 0 || s.DeductionRatio > 100 {
		return ErrInvalidDeductionRatio
	}

	validUnitTimes := []string{UnitTimeNoLimit, UnitTimeYear, UnitTimeMonth, UnitTimeDay, UnitTimeHour, UnitTimeMinute}
	valid := false
	for _, ut := range validUnitTimes {
		if s.UnitTime == ut {
			valid = true
			break
		}
	}
	if !valid {
		return ErrInvalidUnitTime
	}

	return nil
}

// Validate checks if the Order struct contains valid data
func (o *Order) Validate() error {
	if o.Amount < 0 {
		return ErrInvalidAmount
	}
	return nil
}

// CalculateRemainingAmount returns the refundable share of what was paid for
// the subscription. Time after the current traffic-reset cycle refunds pro
// rata; the current cycle refunds by its unused time and traffic. The result
// is always between zero and the amount paid.
func CalculateRemainingAmount(sub Subscribe, order Order) (int64, error) {
	return calculateRemainingAmount(sub, order, timeutil.Now())
}

func calculateRemainingAmount(sub Subscribe, order Order, now time.Time) (int64, error) {
	if err := sub.Validate(); err != nil {
		return 0, fmt.Errorf("invalid subscription: %w", err)
	}

	if err := order.Validate(); err != nil {
		return 0, fmt.Errorf("invalid order: %w", err)
	}

	if sub.UnitTime == UnitTimeNoLimit {
		if sub.ResetCycle != ResetCycleNone {
			return 0, nil
		}
		return calculateNoLimitAmount(sub, order), nil
	}

	if order.Amount == 0 || !now.Before(sub.ExpireTime) {
		return 0, nil
	}
	if now.Before(sub.StartTime) {
		now = sub.StartTime
	}

	cycleStart, cycleEnd := currentResetCycle(sub, now)
	remaining := float64(sub.ExpireTime.Sub(cycleEnd))
	if cycle := cycleEnd.Sub(cycleStart); cycle > 0 {
		remaining += float64(cycle) * remainingShare(sub, float64(cycleEnd.Sub(now))/float64(cycle))
	}
	return proportion(order.Amount, remaining/float64(sub.ExpireTime.Sub(sub.StartTime))), nil
}

// calculateNoLimitAmount refunds a time-unlimited subscription by its unused
// traffic; without a traffic cap there is nothing left to measure.
func calculateNoLimitAmount(sub Subscribe, order Order) int64 {
	if sub.Traffic == 0 {
		return 0
	}
	return proportion(order.Amount, float64(sub.Traffic-sub.Download-sub.Upload)/float64(sub.Traffic))
}

// currentResetCycle returns the traffic-reset cycle containing now, clamped
// to the subscription term. It follows the reset job's calendar: the 1st of
// each month, the start day of each month (the last day in shorter months),
// or the start date each year (Feb 28 for a Feb 29 start in common years).
// Without a reset cycle the whole term is one cycle.
func currentResetCycle(sub Subscribe, now time.Time) (time.Time, time.Time) {
	loc := sub.StartTime.Location()
	now = now.In(loc)
	start := sub.StartTime

	var cycleStart, cycleEnd time.Time
	switch sub.ResetCycle {
	case ResetCycle1st:
		cycleStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		cycleEnd = cycleStart.AddDate(0, 1, 0)
	case ResetCycleMonthly:
		cycleStart = resetDate(now.Year(), now.Month(), start.Day(), loc)
		if cycleStart.After(now) {
			cycleStart = resetDate(now.Year(), now.Month()-1, start.Day(), loc)
		}
		cycleEnd = resetDate(cycleStart.Year(), cycleStart.Month()+1, start.Day(), loc)
	case ResetCycleYear:
		cycleStart = resetDate(now.Year(), start.Month(), start.Day(), loc)
		if cycleStart.After(now) {
			cycleStart = resetDate(now.Year()-1, start.Month(), start.Day(), loc)
		}
		cycleEnd = resetDate(cycleStart.Year()+1, start.Month(), start.Day(), loc)
	default:
		return sub.StartTime, sub.ExpireTime
	}

	if cycleStart.Before(sub.StartTime) {
		cycleStart = sub.StartTime
	}
	if cycleEnd.After(sub.ExpireTime) {
		cycleEnd = sub.ExpireTime
	}
	return cycleStart, cycleEnd
}

// resetDate returns local midnight of the given day, moved to the month's
// last day when the month is shorter. Months outside 1-12 roll over into the
// neighboring year.
func resetDate(year int, month time.Month, day int, loc *time.Location) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	if last := first.AddDate(0, 1, -1).Day(); day > last {
		day = last
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, loc)
}

// remainingShare combines the unused time and traffic of the current cycle
// the way the plan asks: the smaller of the two by default, or a weighted mix
// when the plan sets a deduction ratio.
func remainingShare(sub Subscribe, timeRatio float64) float64 {
	if sub.Traffic == 0 {
		return timeRatio
	}
	trafficRatio := float64(sub.Traffic-sub.Download-sub.Upload) / float64(sub.Traffic)
	if sub.DeductionRatio == 0 {
		return math.Min(timeRatio, trafficRatio)
	}
	trafficWeight, timeWeight := calculateWeights(sub.DeductionRatio)
	return trafficWeight*trafficRatio + timeWeight*timeRatio
}

// calculateWeights converts deduction ratio to traffic and time weights
// for weighted calculations
func calculateWeights(deductionRatio int64) (float64, float64) {
	if deductionRatio == 0 {
		return 0, 0
	}
	trafficWeight := float64(deductionRatio) / 100
	timeWeight := 1 - trafficWeight
	return trafficWeight, timeWeight
}

// proportion returns the given share of amount, clamped to [0, amount].
func proportion(amount int64, share float64) int64 {
	switch {
	case math.IsNaN(share) || share <= 0:
		return 0
	case share >= 1:
		return amount
	}
	return int64(float64(amount) * share)
}
