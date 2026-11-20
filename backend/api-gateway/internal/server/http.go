package server

import (
	stdhttp "net/http"

	v1 "api-gateway/api/helloworld/v1"
	"api-gateway/internal/conf"
	"api-gateway/internal/router"
	"api-gateway/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, greeter *service.GreeterService, logger log.Logger) *kratoshttp.Server {
	if c == nil {
		c = &conf.Server{}
	}
	if c.Http == nil {
		c.Http = &conf.Server_HTTP{}
	}

	var opts = []kratoshttp.ServerOption{
		kratoshttp.Filter(router.CorrelationID(logger)),
		kratoshttp.Middleware(
			recovery.Recovery(),
		),
		kratoshttp.NotFoundHandler(stdhttp.NotFoundHandler()),
		kratoshttp.MethodNotAllowedHandler(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
			w.WriteHeader(stdhttp.StatusMethodNotAllowed)
		})),
	}
	if c.Http.Network != "" {
		opts = append(opts, kratoshttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratoshttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, kratoshttp.Timeout(c.Http.Timeout.AsDuration()))
	}

	srv := kratoshttp.NewServer(opts...)
	v1.RegisterGreeterHTTPServer(srv, greeter)

	// Configure rate limiter
	limiter, rps := router.NewRateLimiter()

	// Register all routes using the router module
	router.RegisterHealthEndpoints(srv)
	router.RegisterSwaggerEndpoints(srv)
	router.RegisterProxyRoutes(srv, limiter, rps)

	return srv
}
