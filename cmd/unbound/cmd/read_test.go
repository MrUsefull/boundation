package cmd

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/MrUsefull/boundation/internal/unbound"
	"github.com/MrUsefull/boundation/internal/unbound/testhelpers"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/external-dns/endpoint"
)

func Test_printEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		endpoints []*endpoint.Endpoint
		want      string
	}{
		{
			name: "Nil endpoints",
			want: "\nDNS Name     Target     Record Type     \n",
		},
		{
			name:      "empty endpoints",
			want:      "\nDNS Name     Target     Record Type     \n",
			endpoints: []*endpoint.Endpoint{},
		},
		{
			name: "some endpoints",
			want: `
DNS Name        Target      Record Type     
foo.bar.baz     1.2.3.4     AAAA
fizz.buzz       5.6.7.8     A
`,
			endpoints: []*endpoint.Endpoint{
				{
					DNSName:    "foo.bar.baz",
					Targets:    []string{"1.2.3.4"},
					RecordType: endpoint.RecordTypeAAAA,
				},
				{
					DNSName:    "fizz.buzz",
					Targets:    []string{"5.6.7.8"},
					RecordType: endpoint.RecordTypeA,
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := &bytes.Buffer{}
			printEndpoints(w, tt.endpoints)
			assert.Equal(t, tt.want, w.String())
		})
	}
}

func Test_readEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cmd     *cobra.Command
		handler http.HandlerFunc
		wantOut string
		wantErr error
	}{
		{
			name: "Simple Success",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				return cmd
			}(),
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.NotNil(t, r.Body)
				body := `{"Rows": [{
        "uuid": "some-uuid-here",
        "hostname": "foo",
        "domain": "example.domain",
        "rr": "A (Ipv4 Address)",
        "Server": "10.0.0.4",
        "Description": ""
	}]
  }`
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(body))
				assert.NoError(t, err)
			},
			wantOut: `
DNS Name               Target       Record Type     
foo.example.domain     10.0.0.4     A
`,
		},
		{
			name: "Read Failure",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				return cmd
			}(),
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.NotNil(t, r.Body)
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: unbound.ErrRequestFailed,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			outWriter := &bytes.Buffer{}
			testServe := testhelpers.ServerForTest(t, tt.handler)
			err := readEndpoints(testServe.Client(), testServe.Config(), outWriter, tt.cmd)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantOut, outWriter.String())
		})
	}
}
