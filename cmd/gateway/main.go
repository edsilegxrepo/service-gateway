package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edsilegxrepo/service-gateway/internal/auth"
	"github.com/edsilegxrepo/service-gateway/internal/config"
	"github.com/edsilegxrepo/service-gateway/internal/crypto"
	"github.com/edsilegxrepo/service-gateway/internal/gateway"
	"github.com/edsilegxrepo/service-gateway/internal/logging"
	"github.com/edsilegxrepo/service-gateway/internal/platform"
	"github.com/edsilegxrepo/service-gateway/internal/services"

	_ "github.com/edsilegxrepo/service-gateway/internal/services/ldap"
	_ "github.com/edsilegxrepo/service-gateway/internal/services/mongodb"
	_ "github.com/edsilegxrepo/service-gateway/internal/services/mq"
	_ "github.com/edsilegxrepo/service-gateway/internal/services/o365mbx"
	_ "github.com/edsilegxrepo/service-gateway/internal/services/pdfextractor"
	_ "github.com/edsilegxrepo/service-gateway/internal/services/rclone"
	_ "github.com/edsilegxrepo/service-gateway/internal/services/sql"
	_ "github.com/edsilegxrepo/service-gateway/internal/services/wrapper"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	showVersion := flag.Bool("version", false, "Show version and exit")
	keyGen := flag.String("key-gen", "", "Generate a new master key and save to path")
	flag.Parse()

	if *showVersion {
		fmt.Printf("service-gateway %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	if *keyGen != "" {
		if err := generateMasterKey(*keyGen); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating key: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Master key generated: %s\n", *keyGen)
		os.Exit(0)
	}

	if err := run(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := logging.New(cfg.Logging)
	slog.SetDefault(logger)

	if err := crypto.InitMasterKey(cfg.Auth.MasterKeyPath, cfg.Auth.MasterKeyEnvVar); err != nil {
		return fmt.Errorf("init master key: %w", err)
	}

	plat := platform.Current()
	if cfg.Security.LockMemory {
		if err := plat.LockMemory(); err != nil {
			logger.Warn("could not lock memory", "error", err)
		}
	}

	registry := services.NewRegistry(logger)
	if err := registry.LoadServices(cfg.ServicesDir); err != nil {
		return fmt.Errorf("load services: %w", err)
	}

	authMiddleware := auth.NewMiddleware(cfg.Auth, logger)
	nonceStore := auth.NewNonceStore(cfg.Auth.NonceTTL)

	gw := gateway.New(gateway.Config{
		Registry:    registry,
		Auth:        authMiddleware,
		NonceStore:  nonceStore,
		Logger:      logger,
		StartTime:   time.Now(),
		Version:     version,
		ServicesDir: cfg.ServicesDir,
		ReloadFn:    registry.Reload,
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      gw.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		logger.Info("starting server", "addr", addr, "version", version)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	}()

	if reloadCh := plat.SetupReloadSignal(); reloadCh != nil {
		go func() {
			for range reloadCh {
				logger.Info("reloading configuration")
				if err := registry.Reload(); err != nil {
					logger.Error("reload failed", "error", err)
				}
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down, draining requests...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	registry.CloseAll()
	logger.Info("shutdown complete")

	return nil
}

func generateMasterKey(path string) error {
	keyHex, err := crypto.GenerateKey()
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(keyHex), 0o600); err != nil {
		return err
	}

	return platform.Current().SecureFilePermissions(path)
}
