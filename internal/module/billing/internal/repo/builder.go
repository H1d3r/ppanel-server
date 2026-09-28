package repo

import "github.com/perfect-panel/server/internal/repository"

// NewBuilder assembles the billing repository bundle over a module
// connection; the module facade exports it for store assembly.
func NewBuilder() repository.BillingBuilder {
	return func(c repository.ModuleConn) repository.BillingRepos {
		conn := c.Conn()
		wallets := NewWalletRepo(conn)
		orders := NewOrderRepo(conn)
		return repository.BillingRepos{
			Orders:      orders,
			OrderEvents: NewOrderEventRepo(c.DB),
			Payments:    NewPaymentRepo(conn),
			Coupons:     NewCouponRepo(conn),
			Withdrawals: wallets,
			Wallets:     wallets,
			OrderStats:  orders.(repository.OrderStatsBridge),
		}
	}
}
