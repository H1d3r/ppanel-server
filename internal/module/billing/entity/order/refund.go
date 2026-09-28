package order

// RefundBasis is what was paid for the subscription term: the original order
// plus its paid renewals. Traffic resets buy traffic, not time, and are not
// refunded. The refund quote and the refund settlement both measure against
// it.
func (d *Details) RefundBasis() int64 {
	basis := d.Amount + d.GiftAmount
	for _, subOrder := range d.SubOrders {
		if subOrder.IsPaidRenewal() {
			basis += subOrder.Amount + subOrder.GiftAmount
		}
	}
	return basis
}

// IsPaidRenewal reports whether the order renews a subscription and its
// payment was collected.
func (o *Order) IsPaidRenewal() bool {
	return o.Type == TypeRenewal && IsSettled(o.Status)
}
