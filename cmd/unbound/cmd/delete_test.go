package cmd

import (
	"context"
	"net/http"
	"testing"

	"github.com/MrUsefull/boundation/internal/unbound"
	"github.com/MrUsefull/boundation/internal/unbound/testhelpers"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_hostsToEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		hosts []string
		want  map[string]struct{}
	}{
		{
			name: "nil input",
			want: make(map[string]struct{}),
		},
		{
			name:  "empty input",
			hosts: make([]string, 0),
			want:  make(map[string]struct{}),
		},
		{
			name:  "some hosts",
			hosts: []string{"host1.example.com", "hosty.mchost.face"},
			want: map[string]struct{}{
				"host1.example.com": {},
				"hosty.mchost.face": {},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toSet(tt.hosts)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_parseDeleteFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cmd     *cobra.Command
		want    map[string]struct{}
		wantErr error
	}{
		{
			name: "Happy path",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				setDeleteCmdFlags(cmd)
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host1.com"))
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host2.com"))
				return cmd
			}(),
			want: map[string]struct{}{
				"host1.com": {},
				"host2.com": {},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDeleteFlags(tt.cmd)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func Test_deleteEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cmd     *cobra.Command
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name: "Apply fails",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				setDeleteCmdFlags(cmd)
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host1.com"))
				return cmd
			}(),
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "boink", http.StatusInternalServerError)
			},
			wantErr: unbound.ErrRequestFailed,
		},
		{
			name: "Simple Success",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				setDeleteCmdFlags(cmd)
				require.NoError(t, cmd.Flags().Set(hostsFlag, "host1.com"))
				return cmd
			}(),
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.NotNil(t, r.Body)
				_, err := w.Write([]byte(testhelpers.DeleteSuccessServResp))
				assert.NoError(t, err)
			},
		},
		{
			name: "Bad cmd input: Missing hosts",
			cmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.SetContext(context.Background())
				setDeleteCmdFlags(cmd)
				return cmd
			}(),
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.NotNil(t, r.Body)
				_, err := w.Write([]byte(testhelpers.DeleteSuccessServResp))
				assert.NoError(t, err)
			},
			wantErr: ErrMissingHosts,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testServe := testhelpers.ServerForTest(t, tt.handler)
			err := deleteEndpoints(testServe.Client(), testServe.Config(), tt.cmd)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
