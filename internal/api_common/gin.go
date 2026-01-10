package api_common

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func GinForService(service config.Service) *gin.Engine {
	logFormatter := func(param gin.LogFormatterParams) string {
		var statusColor, methodColor, resetColor string
		if param.IsOutputColor() {
			statusColor = param.StatusCodeColor()
			methodColor = param.MethodColor()
			resetColor = param.ResetColor()
		}

		if param.Latency > time.Minute {
			param.Latency = param.Latency.Truncate(time.Second)
		}
		return fmt.Sprintf("["+string(service.GetId())+"] %v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			statusColor, param.StatusCode, resetColor,
			param.Latency,
			param.ClientIP,
			methodColor, param.Method, resetColor,
			param.Path,
			param.ErrorMessage,
		)
	}

	engine := gin.New()
	engine.Use(gin.LoggerWithFormatter(logFormatter), gin.Recovery())

	return engine
}

// RunServer Runs a HTTP server and handles termination signals automatically.
func RunServer(srv *http.Server, logger *slog.Logger) (err error) {
	// Create channel to listen for signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		var err error
		if srv.TLSConfig != nil {
			logger.Info("Starting Gin server with TLS...")
			err = srv.ListenAndServeTLS("", "")
		} else {
			logger.Info("Starting Gin server...")
			err = srv.ListenAndServe()
		}

		if err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
				return
			}
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal
	<-quit
	logger.Info("Shutting down Gin server...")

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	logger.Info("Gin Server exiting")

	return
}
