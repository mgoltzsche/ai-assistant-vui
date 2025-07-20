package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mgoltzsche/ai-assistant-vui/internal/cli"
	"github.com/mgoltzsche/ai-assistant-vui/internal/server"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tlsutils"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

func main() {
	configFile := "/etc/ai-assistant-vui/config.yaml"
	cfg, err := config.FromFile(configFile)
	configFlag := &config.Flag{File: configFile, Config: &cfg}

	listenAddr := ":8443"
	webDir := "/var/lib/ai-assistant-vui/ui"
	tlsEnabled := false
	tlsCert := ""
	tlsKey := ""

	flag.Var(configFlag, "config", "Path to the configuration file")
	flag.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "URL pointing to the OpenAI API server that runs the LLM")
	flag.StringVar(&listenAddr, "listen", listenAddr, "Address the server should listen on")
	flag.StringVar(&webDir, "web-dir", webDir, "Path to the web UI directory")
	flag.BoolVar(&tlsEnabled, "tls", tlsEnabled, "Serve securely via HTTPS/TLS")
	flag.StringVar(&tlsKey, "tls-key", tlsKey, "Path to the TLS key file")
	flag.StringVar(&tlsCert, "tls-cert", tlsKey, "Path to the TLS certificate file")
	cli.ParseFlagsWithEnvVars(flag.CommandLine, "VUI_")

	if !configFlag.IsSet && err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err = runServer(ctx, cfg, listenAddr, webDir, tlsEnabled, tlsCert, tlsKey)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func runServer(ctx context.Context, cfg config.Configuration, listenAddr, webDir string, tlsEnabled bool, tlsCert, tlsKey string) error {
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:        listenAddr,
		BaseContext: func(net.Listener) context.Context { return ctx },
		Handler:     mux,
	}

	server.AddRoutes(ctx, cfg, webDir, mux)

	go func() {
		<-ctx.Done()
		slog.Info("terminating")
		srv.Shutdown(ctx)
	}()

	var err error

	if tlsEnabled {
		//srv.TLSConfig = &tls.Config{}
		if tlsCert == "" && tlsKey == "" {
			slog.Info("generating self-signed TLS certificate")

			var cleanup func()

			tlsCert, tlsKey, cleanup, err = tlsutils.GenerateSelfSignedTLSCertificate()
			if err != nil {
				return fmt.Errorf("generating tls certificate: %w", err)
			}

			defer cleanup()
		}

		slog.Info(fmt.Sprintf("listening on %s", srv.Addr))

		err = srv.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		slog.Info(fmt.Sprintf("listening on %s", srv.Addr))

		err = srv.ListenAndServe()
	}
	if err == http.ErrServerClosed {
		return nil
	}

	return err
}
