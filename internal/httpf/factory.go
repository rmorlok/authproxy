package httpf

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
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
	additionalMiddlewares ...RoundTripperFactory,
) F {
	// Order matters here to determine the order of middlewares.
	// Request logging is outermost so all requests (including rate-limited ones) are logged.
	middlewares := []RoundTripperFactory{requestLog}
	for _, m := range additionalMiddlewares {
		if m != nil {
			middlewares = append(middlewares, m)
		}
	}

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

	if connLabels := c.GetLabels(); len(connLabels) > 0 {
		ri.Labels = make(map[string]string, len(connLabels))
		for k, v := range connLabels {
			ri.Labels[k] = v
		}
	}

	if rlp, ok := c.(RateLimitConfigProvider); ok {
		ri.RateLimiting = rlp.GetRateLimitConfig()
	}

	return fp.ForRequestInfo(ri)
}

func (f *clientFactory) ForLabels(labels map[string]string) F {
	if len(labels) == 0 {
		return f
	}

	ri := f.requestInfo
	if ri.Labels == nil {
		ri.Labels = make(map[string]string, len(labels))
	} else {
		// Copy existing labels so we don't mutate shared state
		newLabels := make(map[string]string, len(ri.Labels)+len(labels))
		for k, v := range ri.Labels {
			newLabels[k] = v
		}
		ri.Labels = newLabels
	}
	// Request labels override connection user labels — but apxy/ keys are
	// system-managed and per-request input may not modify them.
	// ProxyRequest.Validate already rejects apxy/ keys; this is
	// defense-in-depth against internal callers that might bypass it.
	for k, v := range labels {
		if strings.HasPrefix(k, database.ApxyReservedPrefix) {
			continue
		}
		ri.Labels[k] = v
	}

	return f.ForRequestInfo(ri)
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
