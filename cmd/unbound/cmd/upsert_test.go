package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/MrUsefull/boundation/internal/unbound"
	"github.com/MrUsefull/boundation/internal/unbound/testhelpers"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
)

func Test_toEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   map[string]string
		want []*endpoint.Endpoint
	}{
		{
			name: "Simple test",
			in: map[string]string{
				"host1.fqdn": "1.2.3.4",
				"host2.fqdn": "5.6.7.8",
			},
			want: []*endpoint.Endpoint{
				endpoint.NewEndpoint("host1.fqdn", endpoint.RecordTypeA, "1.2.3.4"),
				endpoint.NewEndpoint("host2.fqdn", endpoint.RecordTypeA, "5.6.7.8"),
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toEndpoints(tt.in)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func Test_toMapping(t *testing.T) {
	t.Parallel()
	type args struct {
		hosts   []string
		targets []string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr error
	}{
		{
			name: "Mismatched",
			args: args{
				hosts: []string{"host1.fqdn"},
				// note the missing target
				targets: []string{},
			},
			wantErr: ErrUnequalHostTargets,
		},
		{
			name: "happy path",
			args: args{
				hosts: []string{"host1.fqdn", "host2.fqdn"},
				// note the missing target
				targets: []string{"1.2.3.4", "5.6.7.8"},
			},
			want: map[string]string{
				"host1.fqdn": "1.2.3.4",
				"host2.fqdn": "5.6.7.8",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := toMapping(tt.args.hosts, tt.args.targets)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func Test_parseHostMappings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cmd     *cobra.Command
		want    map[string]string
		wantErr error
	}{
		{
			name: "simple test",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				setCreateCmdFlags(cmd)
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host1.com"))
				require.NoError(t, cmd.Flags().Set(targetsFlag, "1.2.3.4"))
				return cmd
			}(),
			want: map[string]string{
				"host1.com": "1.2.3.4",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseHostMappings(tt.cmd)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func Test_create_doUpsert(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		serveResponses []string
		wantRequests   map[string][]string
		cmd            *cobra.Command
		wantErr        bool
	}{
		{
			name: "happy path - must be created",
			serveResponses: []string{
				requireGenerateReadResponse(t, []unbound.Record{
					{
						UUID:     "some-uuid-here",
						Hostname: "otherhost",
						Domain:   "awesomepossom.fqdn",
						Rr:       "A",
						Server:   "1.2.3.4",
						Enabled:  "1",
					},
				}),
				testhelpers.CreateSuccessServResp,
				testhelpers.ReconfigureResp,
			},
			wantRequests: map[string][]string{
				unbound.SearchOverridesEndpoint: {""},
				unbound.AddOverrideEndpoint: {
					`{"host":{"hostname":"host1","domain":"com","rr":"A","server":"1.2.3.4","enabled":"1","description":"Managed by K8s external-dns "}}`, //nolint
				},
				unbound.ApplyChangesEndpoint: {`"{}"`},
			},
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				setCreateCmdFlags(cmd)
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host1.com"))
				require.NoError(t, cmd.Flags().Set(targetsFlag, "1.2.3.4"))
				return cmd
			}(),
		},
		{
			name: "happy path - already exists",
			serveResponses: []string{
				requireGenerateReadResponse(t, []unbound.Record{
					{
						UUID:     "some-uuid-here",
						Hostname: "host1",
						Domain:   "domain.com",
						Rr:       "A",
						Server:   "1.2.3.4",
						Enabled:  "1",
					},
				}),
			},
			wantRequests: map[string][]string{
				unbound.SearchOverridesEndpoint: {""},
			},
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				setCreateCmdFlags(cmd)
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host1.domain.com"))
				require.NoError(t, cmd.Flags().Set(targetsFlag, "1.2.3.4"))
				return cmd
			}(),
		},
		{
			name: "happy path - must be updated",
			serveResponses: []string{
				requireGenerateReadResponse(t, []unbound.Record{
					{
						UUID:     "some-uuid-here",
						Hostname: "host1",
						Domain:   "domain.com",
						Rr:       "A",
						Server:   "5.6.7.8",
						Enabled:  "1",
					},
				}),
				requiregenerateOpResponse(t, unbound.DeleteOpSuccessResponse),
				requiregenerateOpResponse(t, unbound.CreateOpSuccessResponse),
				testhelpers.ReconfigureResp,
			},
			wantRequests: map[string][]string{
				fmt.Sprintf("%v%v", unbound.DelOverrideEndpoint, "some-uuid-here"): {`"{}"`},
				unbound.SearchOverridesEndpoint:                                    {""},
				unbound.AddOverrideEndpoint: {
					`{"host":{"hostname":"host1","domain":"domain.com","rr":"A","server":"1.2.3.4","enabled":"1","description":"Managed by K8s external-dns "}}`, //nolint
				},
				unbound.ApplyChangesEndpoint: {`"{}"`},
			},
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				setCreateCmdFlags(cmd)
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host1.domain.com"))
				require.NoError(t, cmd.Flags().Set(targetsFlag, "1.2.3.4"))
				return cmd
			}(),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler, gotRequests := testhelpers.TestHandler(t, tt.serveResponses)
			testServe := testhelpers.ServerForTest(t, handler)
			c := newUpsert(testServe.Client(), testServe.Config(), logger)
			assert.NoError(t, c.doUpsert(tt.cmd))
			assert.Equal(t, tt.wantRequests, gotRequests)
		})
	}
}

func requireGenerateReadResponse(tb testing.TB, hosts []unbound.Record) string {
	tb.Helper()
	shr := unbound.SearchHostResp{
		Rows: hosts,
	}
	out, err := json.Marshal(shr)
	require.NoError(tb, err)
	return string(out)
}

func requiregenerateOpResponse(tb testing.TB, status string) string {
	tb.Helper()
	resp := unbound.OperationResponse{
		Result: status,
	}
	out, err := json.Marshal(resp)
	require.NoError(tb, err)
	return string(out)
}
