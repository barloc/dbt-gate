package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/barloc/dbt-gate/app/api"
	"github.com/barloc/dbt-gate/app/auth"
	"github.com/barloc/dbt-gate/app/gate"
	"github.com/barloc/dbt-gate/app/index"
)

const Name = "dbt-gate"

// Run loads config, starts index refresh and HTTP servers, blocks until signal.
func Run(configPath string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	log, err := newLogger(raw)
	if err != nil {
		return err
	}

	idxCfg, err := index.ParseConfig(raw)
	if err != nil {
		return err
	}
	srvCfg, err := parseServerConfig(raw)
	if err != nil {
		return err
	}

	src, err := auth.Provide(raw)
	if err != nil {
		return err
	}

	store := index.NewStore(idxCfg, log)
	svc := gate.NewService(store, log)
	ctrl := api.NewController(svc, src, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	store.Start(ctx)

	errCh := make(chan error, 2)
	sysSrv := &http.Server{Addr: srvCfg.System.Addr(), Handler: systemHandler()}
	apiSrv := &http.Server{Addr: srvCfg.API.Addr(), Handler: ctrl.Handler()}

	go func() {
		log.Info("system server listening", slog.String("addr", sysSrv.Addr))
		if err := sysSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("system server: %w", err)
		}
	}()
	go func() {
		log.Info("api server listening", slog.String("addr", apiSrv.Addr))
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("api server: %w", err)
		}
	}()

	log.Info("app started", slog.String("product", idxCfg.Product))

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		stop()
		return err
	}

	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = sysSrv.Shutdown(shCtx)
	_ = apiSrv.Shutdown(shCtx)
	log.Info("app stopped")
	return nil
}

func systemHandler() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}).Methods(http.MethodGet)
	r.Handle("/metrics", promhttp.Handler())
	return r
}
