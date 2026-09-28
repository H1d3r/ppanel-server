package dashboard

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

const consoleRevenueStatisticsCacheKey = "console:revenue_statistics"
const consoleRevenueStatisticsCacheTTL = 60 * time.Second

// QueryRevenueStatistics summarizes order revenue for today, this month (with
// its daily breakdown) and all time (with the last months' breakdown). The
// totals are required; a breakdown that cannot be read is left out. The
// summary is cached for a minute.
func (s *Service) QueryRevenueStatistics(ctx context.Context) (*dto.RevenueStatisticsResponse, error) {
	if demoMode() {
		return mockRevenueStatistics(), nil
	}
	if cached, ok := readSnapshot[dto.RevenueStatisticsResponse](ctx, s.deps.Cache, consoleRevenueStatisticsCacheKey); ok {
		return cached, nil
	}

	now := timeutil.Now()
	today, err := s.deps.Orders.QueryDateOrders(ctx, now)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "sum today's orders: %v", err)
	}
	month, err := s.deps.Orders.QueryMonthlyOrders(ctx, now)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "sum this month's orders: %v", err)
	}
	monthly := ordersStatistics("", month)
	monthly.List = make([]dto.OrdersStatistics, 0)
	if days, err := s.deps.Orders.QueryDailyOrdersList(ctx, now); err != nil {
		logger.WithContext(ctx).Errorw("[QueryRevenueStatistics] daily order breakdown", logger.Field("error", err.Error()))
	} else {
		monthly.List = ordersBreakdown(days)
	}

	total, err := s.deps.Orders.QueryTotalOrders(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "sum all orders: %v", err)
	}
	all := ordersStatistics("", total)
	all.List = make([]dto.OrdersStatistics, 0)
	if months, err := s.deps.Orders.QueryMonthlyOrdersList(ctx, now); err != nil {
		logger.WithContext(ctx).Errorw("[QueryRevenueStatistics] monthly order breakdown", logger.Field("error", err.Error()))
	} else {
		all.List = ordersBreakdown(months)
	}

	resp := &dto.RevenueStatisticsResponse{
		Today:   ordersStatistics("", today),
		Monthly: monthly,
		All:     all,
	}
	storeSnapshot(ctx, s.deps.Cache, consoleRevenueStatisticsCacheKey, resp, consoleRevenueStatisticsCacheTTL)
	return resp, nil
}

func ordersStatistics(date string, total order.OrdersTotal) dto.OrdersStatistics {
	return dto.OrdersStatistics{
		Date:               date,
		AmountTotal:        total.AmountTotal,
		NewOrderAmount:     total.NewOrderAmount,
		RenewalOrderAmount: total.RenewalOrderAmount,
	}
}

func ordersBreakdown(periods []order.OrdersTotalWithDate) []dto.OrdersStatistics {
	list := make([]dto.OrdersStatistics, len(periods))
	for i, period := range periods {
		list[i] = ordersStatistics(period.Date, order.OrdersTotal{
			AmountTotal:        period.AmountTotal,
			NewOrderAmount:     period.NewOrderAmount,
			RenewalOrderAmount: period.RenewalOrderAmount,
		})
	}
	return list
}

// mockRevenueStatistics is a mock function to simulate revenue statistics data.
func mockRevenueStatistics() *dto.RevenueStatisticsResponse {
	now := timeutil.Now()

	// Generate daily data for the current month (from 1st to current date)
	monthlyList := make([]dto.OrdersStatistics, 7)
	for i := 0; i < 7; i++ {
		dayDate := now.AddDate(0, 0, -(6 - i))
		baseAmount := int64(25000 + ((6 - i) * 3000) + ((6-i)%3)*8000)
		monthlyList[i] = dto.OrdersStatistics{
			Date:               dayDate.Format("2006-01-02"),
			AmountTotal:        baseAmount,
			NewOrderAmount:     int64(float64(baseAmount) * 0.68),
			RenewalOrderAmount: int64(float64(baseAmount) * 0.32),
		}
	}

	// Generate monthly data for the past 6 months (oldest first)
	allList := make([]dto.OrdersStatistics, 6)
	for i := 0; i < 6; i++ {
		monthDate := now.AddDate(0, -(5 - i), 0)
		baseAmount := int64(1800000 + ((5 - i) * 200000) + ((5-i)%2)*500000)
		allList[i] = dto.OrdersStatistics{
			Date:               monthDate.Format("2006-01"),
			AmountTotal:        baseAmount,
			NewOrderAmount:     int64(float64(baseAmount) * 0.68),
			RenewalOrderAmount: int64(float64(baseAmount) * 0.32),
		}
	}

	return &dto.RevenueStatisticsResponse{
		Today: dto.OrdersStatistics{
			AmountTotal:        35888,
			NewOrderAmount:     22888,
			RenewalOrderAmount: 13000,
		},
		Monthly: dto.OrdersStatistics{
			AmountTotal:        888888,
			NewOrderAmount:     588888,
			RenewalOrderAmount: 300000,
			List:               monthlyList,
		},
		All: dto.OrdersStatistics{
			AmountTotal:        12888888,
			NewOrderAmount:     8588888,
			RenewalOrderAmount: 4300000,
			List:               allList,
		},
	}
}
