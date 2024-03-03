package unbound

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

func Test_cache_updateFromPlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		changes   *plan.Changes
		wantState map[string]string
	}{
		{
			name: "happy path",
			changes: &plan.Changes{
				Create: []*endpoint.Endpoint{
					{
						DNSName:    "a-foo.example.domain",
						RecordType: endpoint.RecordTypeTXT,
						Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"), //nolint:lll
					},
					{
						DNSName:    "foo.example.domain",
						RecordType: endpoint.RecordTypeTXT,
						Targets:    endpoint.NewTargets("heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly"), //nolint:lll
					},
					{
						DNSName:    "foo.example.domain",
						Targets:    endpoint.NewTargets("1.2.3.4"),
						RecordType: endpoint.RecordTypeA,
					},
				},
			},
			wantState: map[string]string{
				"foo.example.domain":   "heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly", //nolint:lll
				"a-foo.example.domain": "heritage=external-dns,external-dns/owner=default,external-dns/resource=ingress/jellybelly/jellybelly", //nolint:lll
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newCache(slog.Default())
			c.updateFromPlan(tt.changes)
			assert.Equal(t, tt.wantState, c.heritages)
		})
	}
}
