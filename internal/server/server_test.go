package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/MrUsefull/boundation/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type testProvider struct {
	recordsResp []*endpoint.Endpoint
	recordsErr  error

	domainFilterResp endpoint.DomainFilter
}

func (tp testProvider) Records(_ context.Context) ([]*endpoint.Endpoint, error) {
	return tp.recordsResp, tp.recordsErr
}

func (tp testProvider) ApplyChanges(_ context.Context, _ *plan.Changes) error {
	return nil
}

func (tp testProvider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	return endpoints, nil
}

func (tp testProvider) GetDomainFilter() endpoint.DomainFilter {
	return tp.domainFilterResp
}

func TestServe(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Listen: config.Listen{
			Addr: "127.0.0.1:1234",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		go server.Serve(ctx, cfg, slog.Default())
	}()

	cancel()
	// just testing we shut down, the rest is tested in other tests
}

func TestServer(t *testing.T) {
	t.Parallel()

	expectedEndpoints := []*endpoint.Endpoint{
		{
			DNSName:    "foo.bar.fqdn",
			RecordType: "A",
			Targets:    []string{"10.11.12.13"},
		},
	}

	cfg := config.Config{
		Listen: config.Listen{
			Addr: "127.0.0.1:6789",
		},
		DomainFilter: config.DomainFilter{
			Filter:  []string{"hello.world"},
			Exclude: []string{"no.touching"},
		},
	}
	provider := testProvider{
		domainFilterResp: endpoint.DomainFilter{
			Filters: cfg.Filter,
		},
		recordsResp: expectedEndpoints,
	}
	subject := server.New(cfg, slog.Default(), server.WithProvider(provider))

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		server.DoServe(ctx, subject)
	}()

	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%v", cfg.Listen.Addr), nil)
		if err != nil {
			return false
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		expectedFilter := &endpoint.DomainFilter{}
		decodeJSON(t, resp.Body, expectedFilter)
		assert.Equal(t, cfg.Filter, expectedFilter.Filters)
		return true
	}, 5*time.Second, 500*time.Millisecond)

	verifyGetRecords(t, cfg, expectedEndpoints)
	verifyAdjustEndpoints(t, cfg, expectedEndpoints)
	verifyApply(t, cfg, expectedEndpoints)

	cancel()
}

func verifyGetRecords(tb testing.TB, cfg config.Config, expectedEndpoints []*endpoint.Endpoint) {
	tb.Helper()
	ctx := context.Background()
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, fmt.Sprintf("http://%v%v", cfg.Listen.Addr, server.RecordsEndpoint), nil)
	require.NoError(tb, err)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(tb, err)
	defer resp.Body.Close()
	assert.Equal(tb, resp.StatusCode, http.StatusOK)

	assert.Equal(tb, resp.Header.Get("Content-Type"), server.MediaType)

	verifyEndpoints(tb, resp.Body, expectedEndpoints)
}

func verifyAdjustEndpoints(tb testing.TB, cfg config.Config, expectedEndpoints []*endpoint.Endpoint) {
	tb.Helper()
	ctx := context.Background()

	body, err := json.Marshal(expectedEndpoints)
	require.NoError(tb, err)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, fmt.Sprintf("http://%v%v", cfg.Listen.Addr, server.AdjustEndpoint), bytes.NewBuffer(body))
	require.NoError(tb, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(tb, err)
	defer resp.Body.Close()
	assert.Equal(tb, resp.StatusCode, http.StatusOK)

	assert.Equal(tb, resp.Header.Get("Content-Type"), server.MediaType)

	verifyEndpoints(tb, resp.Body, expectedEndpoints)
}

func verifyApply(tb testing.TB, cfg config.Config, expectedEndpoints []*endpoint.Endpoint) {
	tb.Helper()
	ctx := context.Background()

	thePlan := plan.Changes{
		Create: expectedEndpoints,
	}

	body, err := json.Marshal(thePlan)
	require.NoError(tb, err)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, fmt.Sprintf("http://%v%v", cfg.Listen.Addr, server.RecordsEndpoint), bytes.NewBuffer(body))

	require.NoError(tb, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(tb, err)
	defer resp.Body.Close()
	assert.Equal(tb, resp.StatusCode, http.StatusNoContent)
	assert.Equal(tb, resp.Header.Get("Content-Type"), server.MediaType)
}

func verifyEndpoints(tb testing.TB, body io.Reader, expectedEndpoints []*endpoint.Endpoint) {
	tb.Helper()
	endpoints := make([]*endpoint.Endpoint, 0)
	decodeJSON(tb, body, &endpoints)
	assert.Equal(tb, expectedEndpoints, endpoints)
}

func decodeJSON(tb testing.TB, data io.Reader, val interface{}) {
	tb.Helper()
	require.NoError(tb, json.NewDecoder(data).Decode(val))
}
