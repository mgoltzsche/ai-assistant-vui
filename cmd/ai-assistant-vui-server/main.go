package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/mgoltzsche/ai-assistant-vui/internal/server"
	"github.com/mgoltzsche/ai-assistant-vui/pkg/config"
)

func main() {
	configFile := "/etc/ai-assistant-vui/config.yaml"
	cfg, err := config.FromFile(configFile)
	configFlag := &config.Flag{File: configFile, Config: &cfg}

	webDir := "/var/lib/ai-assistant-vui/ui"

	flag.Var(configFlag, "config", "Path to the configuration file")
	flag.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "URL pointing to the OpenAI API server that runs the LLM")
	flag.StringVar(&webDir, "web-dir", webDir, "Path to the web UI directory")
	flag.Parse()

	if !configFlag.IsSet && err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err = runServer(ctx, cfg, webDir)
	if err != nil {
		log.Fatal(err)
	}
}

func runServer(ctx context.Context, cfg config.Configuration, webDir string) error {
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

	log.Println("listening on", srv.Addr)

	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}

	return err
}
