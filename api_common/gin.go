package api_common

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/config"
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
