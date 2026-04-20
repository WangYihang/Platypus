package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/utils/config"
	"github.com/WangYihang/Platypus/internal/utils/update"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

const shutdownTimeout = 30 * time.Second

func main() {
	cfg, configFile, err := loadConfig()
	if err != nil {
		log.Error("config: %v", err)
		os.Exit(1)
	}

	log.Success("Platypus %s is starting...", update.Version)
	log.Success("Using configuration file: %s", configFile)

	core.Ctx = app.New(cfg)
	core.CreateContext()

	if cfg.Update {
		update.ConfirmAndSelfUpdate()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	servers := startHTTPServers(cfg)

	for _, s := range cfg.Servers {
		listener := core.CreateTCPServer(s.Host, uint16(s.Port), s.HashFormat, s.Encrypted, s.DisableHistory, s.PublicIP, s.ShellPath)
		if listener != nil {
			time.Sleep(0x100 * time.Millisecond)
			go (*listener).Run()
		}
	}

	log.Success("Server is running. Use platypus-admin or API to manage. (Ctrl-C to stop)")

	<-ctx.Done()
	log.Info("Shutdown signal received, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("server shutdown: %v", err)
		}
	}
	log.Success("Server stopped cleanly")
}

func loadConfig() (*config.Config, string, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yml")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		return nil, "", fmt.Errorf("read config: %w", err)
	}

	var cfg config.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, v.ConfigFileUsed(), fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validator.New().Struct(&cfg); err != nil {
		return nil, v.ConfigFileUsed(), formatValidationError(err)
	}

	return &cfg, v.ConfigFileUsed(), nil
}

func formatValidationError(err error) error {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err
	}
	msg := "config validation failed:"
	for _, fe := range ve {
		msg += fmt.Sprintf("\n  - %s: %s (got %v)", fe.Namespace(), fe.Tag(), fe.Value())
	}
	return errors.New(msg)
}

func startHTTPServers(cfg *config.Config) []*http.Server {
	var servers []*http.Server

	dh := cfg.Distributor.Host
	dp := cfg.Distributor.Port
	distributor := core.CreateDistributorServer(dh, dp, cfg.Distributor.Url)
	distributorSrv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", dh, dp),
		Handler:           distributor,
		ReadHeaderTimeout: 10 * time.Second,
	}
	servers = append(servers, distributorSrv)

	go func() {
		if err := distributorSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("distributor: %v", err)
		}
	}()
	log.Success("Distributor at: http://%s:%d/", dh, dp)

	if cfg.RESTful.Enable {
		rh := cfg.RESTful.Host
		rp := cfg.RESTful.Port
		rest := api.CreateRESTfulAPIServer()

		auth := api.NewAuth()
		api.RegisterWebSocketRoutes(rest)
		api.RegisterLegacyRoutes(rest, auth)
		api.RegisterV1Routes(rest, auth)

		log.Success("API secret: %s", auth.GetSecret())
		log.Success("  Obtain token: curl -X POST http://%s:%d/api/v1/auth/token -d '{\"secret\":\"%s\"}'", rh, rp, auth.GetSecret())

		restSrv := &http.Server{
			Addr:              fmt.Sprintf("%s:%d", rh, rp),
			Handler:           rest,
			ReadHeaderTimeout: 10 * time.Second,
		}
		servers = append(servers, restSrv)

		go func() {
			if err := restSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("rest: %v", err)
			}
		}()
		log.Success("RESTful API at: http://%s:%d/api/v1/", rh, rp)
		core.Ctx.RESTful = rest
	}

	return servers
}
