package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clipboard-controller/config"
	"clipboard-controller/handler"
	"clipboard-controller/logger"
	"clipboard-controller/middleware"
	"clipboard-controller/service"
	"clipboard-controller/tray"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	Version   = "1.0.0"
	StartTime time.Time
)

// Command line flags
var (
	configPath = flag.String("config", "config.yaml", "Path to config file")
	port       = flag.Int("port", 0, "Server port (overrides config)")
	logLevel   = flag.String("log-level", "", "Log level: debug, info, warn, error (overrides config)")
	logDir     = flag.String("log-dir", "", "Log directory (overrides config)")
	noTray     = flag.Bool("no-tray", false, "Disable system tray (run as console only)")
)

func main() {
	StartTime = time.Now()

	// Parse command line flags
	flag.Parse()

	// Initialize console (needed for -H windowsgui builds)
	if !*noTray {
		tray.InitConsole()
	}

	// Setup zerolog (basic setup first)
	setupLogger()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// Apply command line overrides
	if *port > 0 {
		cfg.Port = *port
	}
	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}
	if *logDir != "" {
		cfg.LogDir = *logDir
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		log.Fatal().Err(err).Msg("Invalid config")
	}

	// Set log level from config
	setLogLevel(cfg.LogLevel)

	log.Info().
		Int("port", cfg.Port).
		Str("log_level", cfg.LogLevel).
		Str("log_dir", cfg.LogDir).
		Msg("Config loaded")

	// Initialize log file manager
	logFileManager, err := logger.NewLogFileManager(cfg.LogDir, cfg.LogRetentionDays)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize log file manager")
	}

	// Initialize event logger
	eventLogger := logger.NewEventLogger(logFileManager)
	eventLogger.SetLogHeartbeats(cfg.LogHeartbeats)

	// Initialize services
	toolRegistry := service.NewToolRegistry(cfg)
	lockManager := service.NewLockManager(cfg, toolRegistry)

	// Set event logger on services
	toolRegistry.SetEventLogger(eventLogger)
	lockManager.SetEventLogger(eventLogger)

	// Metrics provider function for background jobs
	metricsProvider := func() (int, int, string) {
		return toolRegistry.CountOnlineTools(), lockManager.QueueLength(), lockManager.GetCurrentLockHolder()
	}

	// Start service background jobs
	bgJobs := service.NewBackgroundJobs(cfg, toolRegistry, lockManager)
	bgJobs.Start()

	// Start logging background jobs
	ctx, cancelCtx := context.WithCancel(context.Background())
	logBgJobs := logger.NewBackgroundJobs(logFileManager, eventLogger, metricsProvider)
	logBgJobs.Start(ctx)

	// Setup Gin
	if cfg.LogLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	// Add request logging middleware
	if cfg.LogRequests {
		router.Use(middleware.RequestLogger(logFileManager))
	}
	router.Use(consoleRequestLogger())

	// Register handlers
	handler.RegisterHealthHandler(router, Version, &StartTime)
	handler.RegisterToolHandler(router, toolRegistry, cfg)
	handler.RegisterLockHandler(router, lockManager, cfg)
	handler.RegisterConfigHandler(router, cfg)
	handler.RegisterDebugHandler(router, eventLogger, logFileManager)

	// Create HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		log.Info().Int("port", cfg.Port).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Shutdown channel
	shutdown := make(chan struct{})

	// Shutdown function (called by tray or signal)
	doShutdown := func() {
		select {
		case <-shutdown:
			// Already shutting down
			return
		default:
			close(shutdown)
		}
	}

	// Handle OS signals
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		doShutdown()
	}()

	// Run with or without system tray
	if *noTray {
		// Console mode - wait for shutdown signal
		<-shutdown
	} else {
		// System tray mode
		trayApp := tray.New(cfg.Port, doShutdown)

		// Update tray status periodically
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-shutdown:
					return
				case <-ticker.C:
					trayApp.UpdateStatus(toolRegistry.CountOnlineTools(), lockManager.QueueLength())
				}
			}
		}()

		// Run tray (blocking) - exits when user clicks Exit
		trayApp.Run()
	}

	log.Info().Msg("Shutting down server...")

	// Stop background jobs
	bgJobs.Stop()
	logBgJobs.Stop()
	cancelCtx()

	// Close log file manager
	if err := logFileManager.Close(); err != nil {
		log.Error().Err(err).Msg("Error closing log file manager")
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited")
}

func setupLogger() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05.000",
	})
}

func setLogLevel(level string) {
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func consoleRequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		log.Debug().
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Dur("latency", latency).
			Str("client_ip", c.ClientIP()).
			Msg("Request")
	}
}
