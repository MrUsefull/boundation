package testhelpers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	CreateSuccessServResp string = `{"result":"saved"}`
	CreateFailServeResp   string = `{"result":"failure"}`
	DeleteSuccessServResp string = `{"result":"deleted"}`
	DeleteFailServeResp   string = `{"result":"failure"}`
	ReconfigureResp       string = `{"result":""}`
)

type TestServer struct {
	server *httptest.Server
	cfg    config.Config
}

func ServerForTest(t *testing.T, handler http.Handler) *TestServer {
	t.Helper()
	testServe := httptest.NewServer(handler)
	cfg := config.Config{
		Opnsense: config.Opnsense{
			BaseURL: testServe.URL,
			Creds:   "key:secret",
		},
	}
	return &TestServer{
		server: testServe,
		cfg:    cfg,
	}
}

func (ts TestServer) Config() config.Config {
	return ts.cfg
}

func (ts TestServer) Client() *http.Client {
	return ts.server.Client()
}

func TestHandler(tb testing.TB, responses []string) (http.HandlerFunc, map[string][]string) {
	tb.Helper()
	gotRequests := make(map[string][]string)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tb.Helper()
		body := ""
		if r.Body != nil {
			rbody, err := io.ReadAll(r.Body)
			assert.NoError(tb, err)
			body = string(rbody)
		}
		gotRequests[r.URL.Path] = append(gotRequests[r.URL.Path], body)
		resp := responses[0]
		responses = responses[1:]
		_, err := w.Write([]byte(resp))
		require.NoError(tb, err)
	})
	return handler, gotRequests
}
