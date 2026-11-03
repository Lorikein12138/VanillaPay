package app

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/auth"
	"vanillapay/internal/config"
	"vanillapay/internal/device"
	"vanillapay/internal/jobs"
	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/internal/settings"
	"vanillapay/internal/subscription"
)

type Database interface {
	Ping(context.Context) error
	Close()
}

type DatabaseFactory func(context.Context, config.Config) (Database, error)

type ReadinessCheck interface {
	Check(context.Context) error
}

type ReadinessFunc func(context.Context) error

func (f ReadinessFunc) Check(ctx context.Context) error {
	return f(ctx)
}

type Dependencies struct {
	Clock              platform.Clock
	RandomSource       io.Reader
	HTTPClient         platform.SafeHTTPClient
	Mailer             platform.Mailer
	DatabaseFactory    DatabaseFactory
	MigrationReadiness ReadinessCheck
	WorkerReadiness    ReadinessCheck
}

type Services struct {
	Auth         *auth.Service
	Merchant     *merchant.Service
	Settings     *settings.Service
	Subscription *subscription.Service
	Payment      *payment.Service
	Device       *device.Service
	Portal       *portal.Service
	Jobs         *jobs.Registry
}

type App struct {
	config       config.Config
	database     Database
	clock        platform.Clock
	randomSource io.Reader
	httpClient   platform.SafeHTTPClient
	mailer       platform.Mailer
	readiness    []ReadinessCheck
	Services     *Services
	closeOnce    sync.Once
}

// New constructs an application with production adapters.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	return NewWithDependencies(ctx, cfg, Dependencies{})
}

// NewWithDependencies is the constructor-injection point used by local tests
// and by future service composition.
func NewWithDependencies(ctx context.Context, cfg config.Config, deps Dependencies) (*App, error) {
	if cfg.DatabaseURL == "" {
		return nil, errors.New("invalid configuration: DATABASE_URL")
	}
	if cfg.ListenAddr == "" {
		return nil, errors.New("invalid configuration: LISTEN_ADDR")
	}
	if deps.Clock == nil {
		deps.Clock = platform.SystemClock{}
	}
	if deps.RandomSource == nil {
		deps.RandomSource = cryptorand.Reader
	}
	if deps.HTTPClient == nil {
		deps.HTTPClient = platform.PublicHTTPClient{}
	}
	mailerProvided := deps.Mailer != nil
	if deps.Mailer == nil {
		deps.Mailer = platform.DisabledMailer{}
	}
	if deps.DatabaseFactory == nil {
		deps.DatabaseFactory = newPGXPool
	}

	database, err := deps.DatabaseFactory(ctx, cfg)
	if err != nil {
		return nil, errors.New("database pool initialization failed")
	}
	if database == nil {
		return nil, errors.New("database pool initialization failed")
	}

	readiness := make([]ReadinessCheck, 0, 2)
	if deps.MigrationReadiness != nil {
		readiness = append(readiness, deps.MigrationReadiness)
	}
	if deps.WorkerReadiness != nil {
		readiness = append(readiness, deps.WorkerReadiness)
	}

	a := &App{
		config:       cfg,
		database:     database,
		clock:        deps.Clock,
		randomSource: deps.RandomSource,
		httpClient:   deps.HTTPClient,
		mailer:       deps.Mailer,
		readiness:    readiness,
	}
	if pool, ok := database.(*pgxpool.Pool); ok {
		queue := &jobs.RiverQueue{}
		mailer := deps.Mailer
		if !mailerProvided {
			mailer = jobs.QueueMailer{Pool: pool, Key: cfg.EncryptionKey, Random: deps.RandomSource}
			a.mailer = mailer
		}
		billing := payment.NewService(pool, cfg.EncryptionKey, cfg.PublicBaseURL, deps.Clock, deps.RandomSource, nil, queue)
		billing.SetHTTPClient(deps.HTTPClient)
		subs := subscription.NewService(pool, cfg.EncryptionKey, deps.Clock, deps.RandomSource, billing)
		billing.SetFulfillment(subs)
		devices := device.NewService(pool, cfg.EncryptionKey, cfg.PublicBaseURL, deps.Clock, deps.RandomSource, billing)
		identity := auth.NewService(pool, cfg.EncryptionKey, deps.Clock, deps.RandomSource, mailer, auth.Siteverify{})
		merchants := merchant.NewService(pool, cfg.EncryptionKey, deps.Clock, deps.RandomSource, billing, identity)
		merchants.SetPurchasePort(subs)
		merchants.SetPlatformKeyPort(billing)
		portalService := portal.NewService(pool, cfg.EncryptionKey, deps.Clock, deps.RandomSource)
		settingsService := settings.NewService(pool, cfg.EncryptionKey, deps.Clock, deps.RandomSource, cfg.PublicBaseURL, cfg.DataDir)
		registry, err := jobs.NewRegistry(pool, cfg.EncryptionKey, queue, billing, subs, devices, deps.HTTPClient)
		if err != nil {
			database.Close()
			return nil, errors.New("background worker initialization failed")
		}
		a.readiness = append(a.readiness,
			ReadinessFunc(func(ctx context.Context) error {
				var version int64
				if err := pool.QueryRow(ctx, `SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil {
					return err
				}
				if version < 6 {
					return errors.New("application migrations are incomplete")
				}
				return nil
			}),
			ReadinessFunc(func(context.Context) error { return registry.Check() }),
		)
		a.Services = &Services{Auth: identity, Merchant: merchants, Settings: settingsService, Subscription: subs, Payment: billing, Device: devices, Portal: portalService, Jobs: registry}
	}
	return a, nil
}

func (a *App) PublicBaseURL() string {
	if a == nil {
		return ""
	}
	return a.config.PublicBaseURL
}
func (a *App) AppEnvironment() string {
	if a == nil {
		return ""
	}
	return a.config.AppEnv
}
func (a *App) TrustedProxyCIDRs() []netip.Prefix {
	if a == nil {
		return nil
	}
	return append([]netip.Prefix(nil), a.config.TrustedProxyCIDRs...)
}

func newPGXPool(ctx context.Context, cfg config.Config) (Database, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, errors.New("unable to create PostgreSQL pool")
	}
	return pool, nil
}

// Ready checks the database and any configured migration and worker health
// providers. Providers are injected as those systems are added to the app.
func (a *App) Ready(ctx context.Context) error {
	if a == nil || a.database == nil {
		return errors.New("application dependencies unavailable")
	}
	if err := a.database.Ping(ctx); err != nil {
		return errors.New("database unavailable")
	}
	for _, check := range a.readiness {
		if err := check.Check(ctx); err != nil {
			return errors.New("application dependency unavailable")
		}
	}
	return nil
}

// Start serves the supplied handler and gracefully shuts the HTTP server down
// when ctx is cancelled.
func (a *App) Start(ctx context.Context, handler http.Handler) error {
	if a == nil || handler == nil {
		return errors.New("application and HTTP handler are required")
	}
	if a.Services != nil && a.Services.Jobs != nil {
		if err := a.Services.Jobs.Start(ctx); err != nil {
			return fmt.Errorf("start background workers: %w", err)
		}
	}
	listener, err := net.Listen("tcp", a.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on configured address: %w", err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("graceful HTTP shutdown failed: %w", err)
		}
		if a.Services != nil && a.Services.Jobs != nil {
			if err := a.Services.Jobs.Stop(shutdownCtx); err != nil {
				return fmt.Errorf("stop background workers: %w", err)
			}
		}
		if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server stopped unexpectedly: %w", err)
		}
		return nil
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server stopped unexpectedly: %w", err)
	}
}

// Close releases application resources. It is safe to call more than once.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		if a.database != nil {
			a.database.Close()
		}
	})
	return nil
}
