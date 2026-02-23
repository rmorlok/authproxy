package api_common

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

func GinForService(service config.Service, logger *slog.Logger, debugMode bool) *gin.Engine {
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
	engine.Use(DebugModeMiddleware(debugMode), gin.LoggerWithFormatter(logFormatter), gin.Recovery(), ErrorLoggingMiddleware(logger))

	return engine
}

// GinForTest returns a gin engine configured for tests with error logging enabled.
func GinForTest(logger *slog.Logger) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery(), ErrorLoggingMiddleware(logger))
	return engine
}

// DebugModeMiddleware sets the debug mode flag on the request context.
func DebugModeMiddleware(debugMode bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := apctx.WithDebugMode(c.Request.Context(), debugMode)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ErrorLoggingMiddleware logs any request that results in a 500+ response.
// It attempts to include any internal errors attached to the gin context.
func ErrorLoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		status := c.Writer.Status()
		if status < http.StatusInternalServerError {
			return
		}

		if logger == nil {
			if len(c.Errors) == 0 {
				log.Printf("request failed with 5xx status=%d method=%s path=%s", status, c.Request.Method, c.Request.URL.Path)
				return
			}
			for _, err := range c.Errors {
				log.Printf("request failed with 5xx status=%d method=%s path=%s error=%v", status, c.Request.Method, c.Request.URL.Path, err.Err)
			}
			return
		}

		if len(c.Errors) == 0 {
			logger.Error("request failed with 5xx",
				"status", status,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
			)
			return
		}

		for _, err := range c.Errors {
			logger.Error("request failed with 5xx",
				"status", status,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"error", err.Err,
			)
		}
	}
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
