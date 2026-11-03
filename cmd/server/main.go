package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"vanillapay/internal/app"
	"vanillapay/internal/config"
	dbmigrate "vanillapay/internal/db/migrate"
	"vanillapay/internal/httpx"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		var configError *config.ConfigError
		if errors.As(err, &configError) {
			fmt.Fprintf(os.Stderr, "server: %s\n", configError.Error())
		} else {
			fmt.Fprintln(os.Stderr, "server: command failed")
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("expected serve or migrate up")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch args[0] {
	case "serve":
		if len(args) != 1 {
			return errors.New("serve does not accept additional arguments")
		}
		application, err := app.New(ctx, cfg)
		if err != nil {
			return err
		}
		defer func() { _ = application.Close() }() // Preserve the command result during shutdown.
		status, err := application.Services.Auth.BootstrapStatus(ctx)
		if err != nil {
			return err
		}
		if !status["initialized"] {
			code, err := application.Services.Auth.RotateBootstrapCode(ctx)
			if err != nil {
				return err
			}
			if code != "" {
				if _, err := fmt.Fprintf(os.Stdout, "Administrator initialization code: %s\n", code); err != nil {
					return err
				}
			}
		}
		return application.Start(ctx, httpx.NewRouter(application))
	case "console-recovery":
		if len(args) != 1 {
			return errors.New("console-recovery does not accept additional arguments")
		}
		application, err := app.New(ctx, cfg)
		if err != nil {
			return err
		}
		defer func() { _ = application.Close() }() // Preserve the command result during shutdown.
		if application.Services == nil || application.Services.Auth == nil {
			return errors.New("authentication service is unavailable")
		}
		code, err := application.Services.Auth.IssueConsoleRecoveryCode(ctx)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "Console recovery code: %s\n", code)
		return err
	case "migrate":
		if len(args) != 2 || args[1] != "up" {
			return errors.New("expected migrate up")
		}
		return dbmigrate.Up(ctx, cfg.DatabaseURL)
	default:
		return errors.New("expected serve, console-recovery, or migrate up")
	}
}
