package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/subscription"
)

type CallbackArgs struct {
	OrderID int64 `json:"order_id"`
}

func (CallbackArgs) Kind() string { return "vanillapay_callback" }

type CallbackScanArgs struct{}

func (CallbackScanArgs) Kind() string { return "vanillapay_callback_scan" }

type ExpiryArgs struct{}

func (ExpiryArgs) Kind() string { return "vanillapay_expiry_scan" }

type OfflineArgs struct{}

func (OfflineArgs) Kind() string { return "vanillapay_offline_scan" }

type MailArgs struct{}

func (MailArgs) Kind() string { return "vanillapay_mail_scan" }

type RiverQueue struct{ client *river.Client[pgx.Tx] }

func (q *RiverQueue) EnqueueCallbackTx(ctx context.Context, tx pgx.Tx, orderID int64) error {
	if q == nil || q.client == nil {
		return errors.New("river callback queue is unavailable")
	}
	_, err := q.client.InsertTx(ctx, tx, CallbackArgs{OrderID: orderID}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

type Registry struct {
	pool    *pgxpool.Pool
	key     [32]byte
	queue   *RiverQueue
	client  *river.Client[pgx.Tx]
	started bool
}

func NewRegistry(pool *pgxpool.Pool, key [32]byte, queue *RiverQueue, pay *payment.Service, subs *subscription.Service, devices *device.Service, publicBaseURL string) (*Registry, error) {
	if pool == nil || queue == nil {
		return nil, errors.New("river database and queue are required")
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &callbackWorker{service: pay})
	river.AddWorker(workers, &callbackScanWorker{pool: pool, queue: queue})
	river.AddWorker(workers, &expiryWorker{pool: pool, payments: pay, subscriptions: subs})
	river.AddWorker(workers, &offlineWorker{devices: devices})
	river.AddWorker(workers, &mailWorker{pool: pool, key: key, publicBaseURL: publicBaseURL})
	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(15*time.Second), func() (river.JobArgs, *river.InsertOpts) {
			return CallbackScanArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Second}}
		}, &river.PeriodicJobOpts{ID: "vanillapay-callback-scan", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(15*time.Second), func() (river.JobArgs, *river.InsertOpts) {
			return ExpiryArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Second}}
		}, &river.PeriodicJobOpts{ID: "vanillapay-expiry", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(15*time.Second), func() (river.JobArgs, *river.InsertOpts) {
			return OfflineArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Second}}
		}, &river.PeriodicJobOpts{ID: "vanillapay-offline", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(15*time.Second), func() (river.JobArgs, *river.InsertOpts) {
			return MailArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: 15 * time.Second}}
		}, &river.PeriodicJobOpts{ID: "vanillapay-mail", RunOnStart: true}),
	}
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Workers:              workers,
		PeriodicJobs:         periodic,
		RescueStuckJobsAfter: 2 * time.Minute,
		Queues:               map[string]river.QueueConfig{"default": {MaxWorkers: 10}},
	})
	if err != nil {
		return nil, err
	}
	r := &Registry{pool: pool, key: key, queue: queue, client: client}
	queue.client = client
	return r, nil
}

func (r *Registry) Start(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("river client unavailable")
	}
	if r.started {
		return nil
	}
	if err := r.client.Start(ctx); err != nil {
		return err
	}
	r.started = true
	return nil
}
func (r *Registry) Stop(ctx context.Context) error {
	if r == nil || r.client == nil || !r.started {
		return nil
	}
	r.started = false
	return r.client.Stop(ctx)
}
func (r *Registry) Check() error {
	if r == nil || r.client == nil || !r.started {
		return errors.New("river workers are not running")
	}
	return nil
}

type callbackWorker struct {
	river.WorkerDefaults[CallbackArgs]
	service *payment.Service
}

type callbackScanWorker struct {
	river.WorkerDefaults[CallbackScanArgs]
	pool  *pgxpool.Pool
	queue *RiverQueue
}

type expiryWorker struct {
	river.WorkerDefaults[ExpiryArgs]
	pool          *pgxpool.Pool
	payments      *payment.Service
	subscriptions *subscription.Service
}

type offlineWorker struct {
	river.WorkerDefaults[OfflineArgs]
	devices *device.Service
}

type mailWorker struct {
	river.WorkerDefaults[MailArgs]
	pool          *pgxpool.Pool
	key           [32]byte
	publicBaseURL string
}

func (r *Registry) EnqueueMailTx(ctx context.Context, tx pgx.Tx, dedupeKey string) error {
	if r == nil || r.client == nil {
		return errors.New("river mail queue unavailable")
	}
	_, err := r.client.InsertTx(ctx, tx, MailArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}
