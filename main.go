package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/refset/fluxnova-decision-observability/internal/config"
	"github.com/refset/fluxnova-decision-observability/internal/pipeline"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	p, err := pipeline.New(cfg)
	if err != nil {
		log.Fatal("Failed to create pipeline:", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	if err := p.Run(ctx); err != nil {
		log.Fatal("Pipeline error:", err)
	}
}
