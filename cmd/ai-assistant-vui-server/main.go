package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/mgoltzsche/ai-assistant-vui/internal/server"
	"github.com/mgoltzsche/ai-assistant-vui/internal/tlsutils"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

func main() {
	configFile := "/etc/ai-assistant-vui/config.yaml"
	cfg, err := config.FromFile(configFile)
	configFlag := &config.Flag{File: configFile, Config: &cfg}

	webDir := "/var/lib/ai-assistant-vui/ui"
	tlsEnabled := false
	tlsCert := ""
	tlsKey := ""

	flag.Var(configFlag, "config", "Path to the configuration file")
	flag.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "URL pointing to the OpenAI API server that runs the LLM")
	flag.StringVar(&webDir, "web-dir", webDir, "Path to the web UI directory")
	flag.BoolVar(&tlsEnabled, "tls", tlsEnabled, "Serve securely via HTTPS/TLS")
	flag.StringVar(&tlsKey, "tls-key", tlsKey, "Path to the TLS key file")
	flag.StringVar(&tlsCert, "tls-cert", tlsKey, "Path to the TLS certificate file")
	flag.Parse()

	if !configFlag.IsSet && err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err = runServer(ctx, cfg, webDir, tlsEnabled, tlsCert, tlsKey)
	if err != nil {
		log.Fatal("FATAL:", err)
	}
}

func runServer(ctx context.Context, cfg config.Configuration, webDir string, tlsEnabled bool, tlsCert, tlsKey string) error {
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:        ":9090",
		BaseContext: func(net.Listener) context.Context { return ctx },
		Handler:     mux,
	}

	server.AddRoutes(ctx, cfg, webDir, mux)

	go func() {
		<-ctx.Done()
		srv.Shutdown(ctx)
	}()

	var err error

	if tlsEnabled {
		//srv.TLSConfig = &tls.Config{}
		if tlsCert == "" && tlsKey == "" {
			log.Println("generating self-signed TLS certificate")

			var cleanup func()

			tlsCert, tlsKey, cleanup, err = tlsutils.GenerateSelfSignedTLSCertificate()
			if err != nil {
				return fmt.Errorf("generating tls certificate: %w", err)
			}

			defer cleanup()
		}

		log.Println("listening on", srv.Addr)

		err = srv.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		log.Println("listening on", srv.Addr)

		err = srv.ListenAndServe()
	}
	if err == http.ErrServerClosed {
		return nil
	}

	return err
}
