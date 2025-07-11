package main

import (
	"context"
	"dash2hlsd/internal/api"
	"dash2hlsd/internal/channels"
	"dash2hlsd/internal/dash"
	"dash2hlsd/internal/key"
	"dash2hlsd/internal/logger"
	"dash2hlsd/internal/session"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 1. Parse command-line arguments
	listenAddr := flag.String("l", ":8080", "HTTP listen address")
	logLevel := flag.String("L", "info", "Log level (error, warn, info, debug)")
	configFile := flag.String("c", "channels.json", "Path to the channel config file")
	flag.Parse()

	// 2. Initialize logger
	log := logger.NewLogger(*logLevel)
	log.Infof("Starting DASH to HLS Proxy...")
	log.Infof("Log level set to: %s", *logLevel)

	// 3. Load configuration
	cfg, err := channels.LoadConfig(*configFile)
	if err != nil {
		log.Errorf("Failed to load configuration: %v", err)
		os.Exit(1)
	}
	log.Infof("Configuration loaded successfully for: %s", cfg.Name)

	// 4. Initialize services and managers
	dashClient := dash.NewClient(log)
	keyService, err := key.NewService(cfg)
	if err != nil {
		log.Errorf("Failed to initialize key service: %v", err)
		os.Exit(1)
	}
	sessionMgr := session.NewManager(log, cfg, dashClient)

	// The segment cache is created inside the session manager, but we need to start it.
	// This is a bit of a design smell. A better design might have the cache be external.
	// For now, we assume the session manager exposes a way to start its components.
	// Let's add a StartComponents method to SessionManager.
	sessionMgr.Start()

	// 5. Set up API router with dependencies
	router := api.New(sessionMgr, keyService)

	// 6. Set up and run the HTTP server with graceful shutdown
	server := &http.Server{
		Addr:    *listenAddr,
		Handler: router,
	}

	go func() {
		log.Infof("Server starting on %s", *listenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Could not listen on %s: %v\n", *listenAddr, err)
			os.Exit(1)
		}
	}()

	// Listen for shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Infof("Server is shutting down...")

	// Create a context with a timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop background services
	sessionMgr.Stop()

	if err := server.Shutdown(ctx); err != nil {
		log.Errorf("Server shutdown failed: %v", err)
		os.Exit(1)
	}

	log.Infof("Server exited gracefully")
}
