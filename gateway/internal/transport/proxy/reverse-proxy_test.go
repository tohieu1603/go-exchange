package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// httputil.ReverseProxy.ServeHTTP type-asserts CloseNotifier on the writer,
// which panics under httptest.NewRecorder. Use httptest.NewServer for the
// front so a real http.Server provides a fully-featured ResponseWriter.

func newGatewayServer(t *testing.T, target string, prefix string) *httptest.Server {
	t.Helper()
	r := gin.New()
	ProxyGroup(r, prefix, target)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestProxyGroup_ForwardsBodyAndPath(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte("upstream-saw:" + string(body)))
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL, "/api/x")

	resp, err := http.Post(gw.URL+"/api/x/orders", "text/plain", strings.NewReader("payload-123"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, 201, resp.StatusCode, "upstream status should propagate")
	assert.Equal(t, "/api/x/orders", gotPath, "path forwarded verbatim")
	assert.Equal(t, "upstream-saw:payload-123", string(body))
}

func TestProxyGroup_RewritesHostHeader(t *testing.T) {
	// The proxy must set req.Host to the upstream so vhost-based routing works.
	var gotHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL, "/api")

	req, _ := http.NewRequest("GET", gw.URL+"/api/whatever", nil)
	req.Host = "client-supplied.example.com"
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, 200, resp.StatusCode)
	// upstream host is 127.0.0.1:<port> from httptest — must not equal client's.
	assert.NotEqual(t, "client-supplied.example.com", gotHost,
		"client-supplied Host must NOT leak through to upstream")
	assert.Contains(t, gotHost, "127.0.0.1")
}

func TestProxyGroup_RoutesUnderPrefix(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hit:" + r.URL.Path))
	}))
	defer upstream.Close()

	gw := newGatewayServer(t, upstream.URL, "/api/wallet")

	resp, err := http.Get(gw.URL + "/api/wallet/balances")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "hit:/api/wallet/balances", string(body))
}
