package httpf

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/request_log"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gentleman.v2/plugins/transport"
)

type clientFactory struct {
	cfg         config.C
	r           apredis.Client
	logger      *slog.Logger
	requestInfo request_log.RequestInfo

	// Cached at the object level

	toplevel     *gentleman.Client
	topLevelOnce sync.Once
}

func CreateFactory(cfg config.C, r apredis.Client, logger *slog.Logger) F {
	return &clientFactory{
		cfg:    cfg,
		r:      r,
		logger: logger,
		requestInfo: request_log.RequestInfo{
			Namespace: sconfig.RootNamespace,
			Type:      request_log.RequestTypeGlobal,
		},
	}
}

func (f *clientFactory) ForRequestInfo(ri request_log.RequestInfo) F {
	return &clientFactory{
		cfg:         f.cfg,
		r:           f.r,
		logger:      f.logger,
		requestInfo: ri,
	}
}

func (f *clientFactory) ForRequestType(rt request_log.RequestType) F {
	ri := f.requestInfo
	ri.Type = rt

	return f.ForRequestInfo(ri)
}

func (f *clientFactory) ForConnectorVersion(cv ConnectorVersion) F {
	ri := f.requestInfo
	ri.Namespace = cv.GetNamespace()
	ri.ConnectorId = cv.GetId()
	ri.ConnectorVersion = cv.GetVersion()
	ri.ConnectorType = cv.GetType()

	return f.ForRequestInfo(ri)
}

func (f *clientFactory) ForConnection(c Connection) F {
	var fp F = f

	if cg, ok := c.(GettableConnectorVersion); ok {
		cv := cg.GetConnectorVersionEntity()
		fp = fp.ForConnectorVersion(cv)
	}

	ri := f.requestInfo
	ri.ConnectionId = c.GetId()
	ri.Namespace = c.GetNamespace()
	ri.ConnectorId = c.GetConnectorId()
	ri.ConnectorVersion = c.GetConnectorVersion()

	return fp.ForRequestInfo(ri)
}

func (f *clientFactory) New() *gentleman.Client {
	f.topLevelOnce.Do(func() {
		f.toplevel = gentleman.New()

		root := f.cfg.GetRoot()
		if root.HttpLogging.IsEnabled() {
			expiration := root.HttpLogging.GetRetention()
			recordFullRequest := root.HttpLogging.GetFullRequestRecording() == sconfig.FullRequestRecordingAlways
			maxFullRequestSize := root.HttpLogging.GetMaxRequestSize()
			maxFullResponseSize := root.HttpLogging.GetMaxResponseSize()
			maxResponseWait := root.HttpLogging.GetMaxResponseWait()
			fullRequestExpiration := root.HttpLogging.GetFullRequestRetention()

			l := request_log.NewRedisLogger(
				f.r,
				f.logger,
				f.requestInfo,
				expiration,
				recordFullRequest,
				fullRequestExpiration,
				maxFullRequestSize,
				maxFullResponseSize,
				maxResponseWait,
				http.DefaultTransport,
			)

			// Add the Redis transport plugin
			f.toplevel.Use(transport.Set(l))
		}
	})

	return gentleman.New().UseParent(f.toplevel)
}
