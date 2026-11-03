package jobs

import (
	"context"

	"github.com/riverqueue/river"
)

func (w *expiryWorker) Work(ctx context.Context, _ *river.Job[ExpiryArgs]) error {
	if w.payments != nil {
		if _, err := w.payments.ExpireBatch(ctx, 500); err != nil {
			return err
		}
	}
	if w.subscriptions != nil {
		_, err := w.subscriptions.CloseExpiredPurchases(ctx, 500)
		return err
	}
	return nil
}
