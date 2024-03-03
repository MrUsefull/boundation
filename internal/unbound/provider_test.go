package unbound

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/MrUsefull/boundation/internal/unbound/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

func GetTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true}))
}

type response struct {
	body string
	code int
}

//nolint:lll
func TestUnbound_Records(t *testing.T) {
	t.Parallel()

	type fields struct {
		cfg    *config.Config
		logger *slog.Logger
	}

	tests := []struct {
		name       string
		fields     fields
		serverResp response
		want       []*endpoint.Endpoint
		wantErr    error
	}{
		{
			name: "Happy Path",
			fields: fields{
				logger: GetTestLogger(),
				cfg: &config.Config{
					Opnsense: config.Opnsense{
						Creds: "foo:bar",
					},
				},
			},
			serverResp: response{
				body: `{"Rows": [{
        "uuid": "some-uuid-here",
        "hostname": "foo",
        "domain": "example.domain",
        "rr": "A (Ipv4 Address)",
        "Server": "10.0.0.4",
        "Description": "Managed by K8s external-dns aGVyaXRhZ2U9ZXh0ZXJuYWwtZG5zLGV4dGVybmFsLWRucy9vd25lcj1kZWZhdWx0LGV4dGVybmFsLWRucy9yZXNvdXJjZT1pbmdyZXNzL2plbGx5YmVsbHkvamVsbHliZWxseQ=="
	}]
  }`,
				code: http.StatusOK,
			},
			want: []*endpoint.Endpoint{
				{
					DNSName:       "foo.example.domain",
					Targets:       endpoint.NewTargets("10.0.0.4"),
					RecordType:    "A",
					SetIdentifier: "some-uuid-here",
					Labels: map[string]string{
						"description": appendToDescription("aGVyaXRhZ2U9ZXh0ZXJuYWwtZG5zLGV4dGVybmFsLWRucy9vd25lcj1kZWZhdWx0LGV4dGVybmFsLWRucy9yZXNvdXJjZT1pbmdyZXNzL2plbGx5YmVsbHkvamVsbHliZWxseQ=="),
					},
				},
				{
					DNSName:    "foo.example.domain",
					RecordType: endpoint.RecordTypeTXT,
					Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"),
				},
				{
					DNSName:    "a-foo.example.domain",
					RecordType: endpoint.RecordTypeTXT,
					Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"),
				},
			},
		},
		{
			name: "Bad status",
			fields: fields{
				logger: GetTestLogger(),
				cfg: &config.Config{
					Opnsense: config.Opnsense{
						Creds: "foo:bar",
					},
				},
			},
			serverResp: response{
				body: "",
				code: http.StatusBadRequest,
			},
			want:    nil,
			wantErr: ErrRequestFailed,
		},
		{
			name: "Bad Response",
			fields: fields{
				logger: GetTestLogger(),
				cfg: &config.Config{
					Opnsense: config.Opnsense{
						Creds: "foo:bar",
					},
				},
			},
			serverResp: response{
				// intentionally bad
				body: `{"Rows": 
			[
			}`,
				code: http.StatusOK,
			},
			want:    nil,
			wantErr: ErrMarshalling,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.serverResp.code)
				_, err := w.Write([]byte(tt.serverResp.body))
				require.NoError(t, err)
			}))
			defer server.Close()

			cfg := tt.fields.cfg
			cfg.BaseURL = server.URL

			u := New(server.Client(), *cfg, tt.fields.logger)
			got, err := u.Records(context.Background())
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUnbound_ApplyChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		changes      *plan.Changes
		serverResps  []string
		wantErr      error
		wantRequests map[string][]string
	}{
		{
			name:         "Happy Path - no changes",
			changes:      &plan.Changes{},
			serverResps:  []string{},
			wantRequests: map[string][]string{},
		},
		{
			name: "Happy Path - full CRUD",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "create.me",
						Targets:    endpoint.NewTargets("1.2.3.4"),
						RecordType: "A",
					},
					{
						DNSName:    "a-create.me",
						RecordType: endpoint.RecordTypeTXT,
						Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"), //nolint:lll
					},
					{
						DNSName:    "create.me",
						RecordType: endpoint.RecordTypeTXT,
						Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"), //nolint:lll
					},
				},
				UpdateOld: []*endpoint.Endpoint{
					{
						DNSName:       "update.this",
						Targets:       endpoint.NewTargets("4.3.2.1"),
						SetIdentifier: "some-uuid-here",
					},
				},
				UpdateNew: []*endpoint.Endpoint{
					{
						DNSName:       "update.this",
						Targets:       endpoint.NewTargets("4.3.2.1"),
						SetIdentifier: "some-uuid-here",
						RecordType:    "A",
					},
				},
				Delete: []*endpoint.Endpoint{
					{
						DNSName:       "delete.this",
						Targets:       endpoint.NewTargets("5.6.7.8"),
						SetIdentifier: "delete-uuid-goes-here",
					},
				},
			},
			serverResps: []string{
				testhelpers.DeleteSuccessServResp,
				testhelpers.DeleteSuccessServResp,
				testhelpers.CreateSuccessServResp,
				testhelpers.CreateSuccessServResp,
				testhelpers.ReconfigureResp,
			},
			wantRequests: map[string][]string{
				AddOverrideEndpoint: {
					`{"host":{"hostname":"create","domain":"me","rr":"A","server":"1.2.3.4","enabled":"1","description":"Managed by K8s external-dns aGVyaXRhZ2U9ZXh0ZXJuYWwtZG5zLGV4dGVybmFsLWRucy9vd25lcj1kZWZhdWx0LGV4dGVybmFsLWRucy9yZXNvdXJjZT1pbmdyZXNzL2plbGx5YmVsbHkvamVsbHliZWxseQ=="}}`, //nolint:lll
					`{"host":{"hostname":"update","domain":"this","rr":"A","server":"4.3.2.1","enabled":"1","description":"Managed by K8s external-dns "}}`,                                                                                                                                       //nolint:lll
				},
				path.Join(DelOverrideEndpoint, "delete-uuid-goes-here"): {`"{}"`},
				path.Join(DelOverrideEndpoint, "some-uuid-here"):        {`"{}"`},
				ApplyChangesEndpoint: {`"{}"`},
			},
		},
		{
			name: "Create failure",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "create.me",
						Targets:    endpoint.NewTargets("1.2.3.4"),
						RecordType: "A",
					},
					{
						DNSName:    "a-create.me",
						RecordType: endpoint.RecordTypeTXT,
						Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"), //nolint:lll
					},
					{
						DNSName:    "create.me",
						RecordType: endpoint.RecordTypeTXT,
						Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"), //nolint:lll
					},
				},
				UpdateOld: []*endpoint.Endpoint{
					{
						DNSName:       "update.this",
						Targets:       endpoint.NewTargets("4.3.2.1"),
						SetIdentifier: "some-uuid-here",
					},
				},
				UpdateNew: []*endpoint.Endpoint{
					{
						DNSName:       "update.this",
						Targets:       endpoint.NewTargets("4.3.2.1"),
						SetIdentifier: "some-uuid-here",
						RecordType:    "A",
					},
				},
				Delete: []*endpoint.Endpoint{
					{
						DNSName:       "delete.this",
						Targets:       endpoint.NewTargets("5.6.7.8"),
						SetIdentifier: "delete-uuid-goes-here",
					},
				},
			},
			serverResps: []string{
				testhelpers.DeleteSuccessServResp,
				testhelpers.DeleteSuccessServResp,
				testhelpers.CreateFailServeResp,
			},
			wantRequests: map[string][]string{
				path.Join(DelOverrideEndpoint, "delete-uuid-goes-here"): {`"{}"`},
				path.Join(DelOverrideEndpoint, "some-uuid-here"):        {`"{}"`},
				AddOverrideEndpoint: {
					`{"host":{"hostname":"create","domain":"me","rr":"A","server":"1.2.3.4","enabled":"1","description":"Managed by K8s external-dns aGVyaXRhZ2U9ZXh0ZXJuYWwtZG5zLGV4dGVybmFsLWRucy9vd25lcj1kZWZhdWx0LGV4dGVybmFsLWRucy9yZXNvdXJjZT1pbmdyZXNzL2plbGx5YmVsbHkvamVsbHliZWxseQ=="}}`, //nolint:lll
				},
			},
			wantErr: ErrRequestFailed,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			handler, gotRequests := testhelpers.TestHandler(t, tt.serverResps)
			server := httptest.NewServer(handler)
			defer server.Close()
			cfg := config.Config{
				Opnsense: config.Opnsense{
					BaseURL: server.URL,
					Creds:   "apiKey:apiSecret",
				},
			}

			u := New(server.Client(), cfg, GetTestLogger())

			assert.ErrorIs(t, u.ApplyChanges(ctx, tt.changes), tt.wantErr)
			assert.Equal(t, tt.wantRequests, gotRequests)
		})
	}
}

func TestUnbound_GetDomainFilter(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		DomainFilter: config.DomainFilter{
			Filter:  []string{"foo.bar", "baz.biz"},
			Exclude: []string{"example.com"},
		},
	}

	subject := New(http.DefaultClient, cfg, slog.Default())

	want := endpoint.NewDomainFilterWithExclusions(cfg.Filter, cfg.Exclude)

	assert.Equal(t, want, subject.GetDomainFilter())
}

func Test_EndToEnd(t *testing.T) {
	t.Parallel()
	if os.Getenv("TEST_CREDS") == "" || os.Getenv("TEST_URL") == "" {
		t.Skip("Set TEST_CREDS and TEST_URL to run")
	}
	cfg := config.Config{
		Opnsense: config.Opnsense{
			BaseURL: os.Getenv("TEST_URL"),
			Creds:   os.Getenv("TEST_CREDS"),
		},
	}

	subject := New(http.DefaultClient, cfg, slog.Default())

	plan := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "foo.bar",
				Targets:    endpoint.NewTargets("10.11.12.13"),
				RecordType: endpoint.RecordTypeA,
			},
		},
	}
	assert.NoError(t, subject.ApplyChanges(context.Background(), plan))

	got, err := subject.Records(context.Background())
	assert.NoError(t, err)
	assert.Greater(t, len(got), 1)
	assertHasEndpoint(t, "foo.bar", "10.11.12.13", got)

	wantDelete := make([]*endpoint.Endpoint, 0)
	for _, endpoint := range got {
		if strings.HasPrefix(endpoint.Labels["description"], DescriptionPrefix) {
			wantDelete = append(wantDelete, endpoint)
		}
	}

	slog.Warn("found delete endpoints", slog.Any("num", len(wantDelete)))

	assert.NoError(t, subject.deleteEndpoints(context.Background(), wantDelete))
}

func assertHasEndpoint(tb testing.TB, dnsName string, target string, endpoints []*endpoint.Endpoint) {
	tb.Helper()
	for _, ep := range endpoints {
		if ep.DNSName == dnsName && slices.Contains(ep.Targets, target) {
			// found desired condition
			return
		}
	}
	assert.Fail(tb, fmt.Sprintf("failed to find %q -> %q in %q", dnsName, target, endpoints))
}
