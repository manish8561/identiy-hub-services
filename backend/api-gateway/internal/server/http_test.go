package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api-gateway/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNewHTTPServer(t *testing.T) {
	cfg := &conf.Server{Http: &conf.Server_HTTP{Network: "tcp", Addr: ":0", Timeout: durationpb.New(5 * time.Second)}}
	server := NewHTTPServer(cfg, nil, log.DefaultLogger)
	require.NotNil(t, server)
}

func TestNewHTTPServer_PropagatesCorrelationID(t *testing.T) {
	server := NewHTTPServer(&conf.Server{}, nil, log.DefaultLogger)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Correlation-ID", "client-id:123")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, req)

	require.Equal(t, "client-id:123", response.Header().Get("X-Correlation-ID"))
}

func TestNewHTTPServer_GeneratesCorrelationIDForUnknownRoute(t *testing.T) {
	server := NewHTTPServer(&conf.Server{}, nil, log.DefaultLogger)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/not-found", nil))

	require.Equal(t, http.StatusNotFound, response.Code)
	id := response.Header().Get("X-Correlation-ID")
	require.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, id)
}

func TestNewHTTPServer_UsesOptionalHTTPConfig(t *testing.T) {
	cfg := &conf.Server{Http: &conf.Server_HTTP{Network: "tcp"}}
	server := NewHTTPServer(cfg, nil, log.DefaultLogger)
	require.NotNil(t, server)
}

func TestNewHTTPServer_EmptyConfigStillConstructs(t *testing.T) {
	server := NewHTTPServer(&conf.Server{}, nil, log.DefaultLogger)
	require.NotNil(t, server)
}

func TestNewHTTPServer_DoesNotUseDefaultServeMuxForUnknownRoutes(t *testing.T) {
	defaultMux := http.DefaultServeMux
	t.Cleanup(func() {
		http.DefaultServeMux = defaultMux
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/leak", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("private debug endpoint"))
	})
	http.DefaultServeMux = mux

	server := NewHTTPServer(&conf.Server{}, nil, log.DefaultLogger)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/leak", nil))

	require.Equal(t, http.StatusNotFound, response.Code)
	require.NotContains(t, response.Body.String(), "private debug endpoint")
}

func TestNewHTTPServer_DoesNotUseDefaultServeMuxForDisallowedMethods(t *testing.T) {
	defaultMux := http.DefaultServeMux
	t.Cleanup(func() {
		http.DefaultServeMux = defaultMux
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/helloworld/private", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("private debug endpoint"))
	})
	http.DefaultServeMux = mux

	server := NewHTTPServer(&conf.Server{}, nil, log.DefaultLogger)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/helloworld/test", nil))

	require.Equal(t, http.StatusMethodNotAllowed, response.Code)
	require.NotContains(t, response.Body.String(), "private debug endpoint")
}

// Note: Rate limiter and clientIP tests have been moved to the router module
// See internal/router/router_test.go for those tests
