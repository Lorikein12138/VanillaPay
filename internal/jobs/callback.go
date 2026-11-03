package jobs

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

func (w *callbackWorker) Work(ctx context.Context, job *river.Job[CallbackArgs]) error {
	if w.service == nil {
		return errors.New("callback service unavailable")
	}
	return w.service.ProcessCallback(ctx, job.Args.OrderID)
}

func (w *callbackScanWorker) Work(ctx context.Context, _ *river.Job[CallbackScanArgs]) error {
	if w.pool == nil || w.queue == nil || w.queue.client == nil {
		return errors.New("callback scanner is unavailable")
	}
	return pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT o.id FROM callback_events c JOIN orders o ON o.id=c.order_id WHERE c.state IN ('pending','failed','running') AND (c.next_attempt_at IS NULL OR c.next_attempt_at<=now()) AND o.status='paid' AND o.refunded_at IS NULL AND o.notify_url IS NOT NULL AND o.notify_url<>'' ORDER BY o.id LIMIT 200`)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range ids {
			if _, err := w.queue.client.InsertTx(ctx, tx, CallbackArgs{OrderID: id}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}); err != nil {
				return err
			}
		}
		return nil
	})
}
