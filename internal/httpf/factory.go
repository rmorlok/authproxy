package httpf

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gentleman.v2/plugins/transport"
)

type clientFactory struct {
	cfg         config.C
	r           apredis.Client
	middlewares []RoundTripperFactory
	logger      *slog.Logger
	requestInfo RequestInfo

	// Cached at the object level

	factoryParent     *gentleman.Client
	factoryParentOnce sync.Once
}

func CreateFactory(
	cfg config.C,
	r apredis.Client,
	requestLog RoundTripperFactory,
	logger *slog.Logger,
) F {
	// Order matters here to determine the order of middlewares
	middlewares := []RoundTripperFactory{requestLog}

	return &clientFactory{
		cfg:         cfg,
		r:           r,
		middlewares: middlewares,
		logger:      logger,
		requestInfo: RequestInfo{
			Namespace: sconfig.RootNamespace,
			Type:      RequestTypeGlobal,
		},
	}
}

func (f *clientFactory) ForRequestInfo(ri RequestInfo) F {
	return &clientFactory{
		cfg:         f.cfg,
		r:           f.r,
		middlewares: f.middlewares,
		logger:      f.logger,
		requestInfo: ri,
	}
}

func (f *clientFactory) ForRequestType(rt RequestType) F {
	ri := f.requestInfo
	ri.Type = rt

	return f.ForRequestInfo(ri)
}

func (f *clientFactory) ForConnectorVersion(cv ConnectorVersion) F {
	ri := f.requestInfo
	ri.Namespace = cv.GetNamespace()
	ri.ConnectorId = cv.GetId()
	ri.ConnectorVersion = cv.GetVersion()

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
	// Callers use chaining within the factor With(...) structure to
	// define context. By the time they trigger new, the context is established
	// and we can cache with middlewares applied.
	f.factoryParentOnce.Do(func() {
		f.factoryParent = gentleman.New()

		parent := http.DefaultTransport
		for _, m := range f.middlewares {
			result := m.NewRoundTripper(f.requestInfo, parent)
			if result != nil {
				parent = result
			}
		}

		f.factoryParent.Use(
			transport.Set(parent),
		)
	})

	return gentleman.New().UseParent(f.factoryParent)
}
