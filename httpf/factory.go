package httpf

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/rmorlok/authproxy/apredis"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/request_log"
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
			Type: request_log.RequestTypeGlobal,
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
	ri.ConnectorId = cv.GetID()
	ri.ConnectorVersion = cv.GetVersion()
	ri.ConnectorType = cv.GetType()

	return f.ForRequestInfo(ri)
}

func (f *clientFactory) ForConnection(cv Connection) F {
	ri := f.requestInfo
	ri.ConnectionId = cv.GetID()
	ri.ConnectorId = cv.GetConnectorId()
	ri.ConnectorVersion = cv.GetConnectorVersion()

	return f.ForRequestInfo(ri)
}

func (f *clientFactory) New() *gentleman.Client {
	f.topLevelOnce.Do(func() {
		f.toplevel = gentleman.New()

		root := f.cfg.GetRoot()
		if root.HttpLogging.IsEnabled() {
			expiration := root.HttpLogging.GetRetention()
			recordFullRequest := root.HttpLogging.GetFullRequestRecording() == config.FullRequestRecordingAlways
			maxFullRequestSize := root.HttpLogging.GetMaxRequestSize()
			maxFullResponseSize := root.HttpLogging.GetMaxResponseSize()
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
				http.DefaultTransport,
			)

			// Add the Redis transport plugin
			f.toplevel.Use(transport.Set(l))
		}
	})

	return gentleman.New().UseParent(f.toplevel)
}
