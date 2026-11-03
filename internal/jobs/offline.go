package jobs

import (
	"context"
	"errors"

	"github.com/riverqueue/river"
)

func (w *offlineWorker) Work(ctx context.Context, _ *river.Job[OfflineArgs]) error {
	if w.devices == nil {
		return errors.New("device service unavailable")
	}
	_, err := w.devices.DetectOffline(ctx, 500)
	return err
}
