package config

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testYaml string = `---
opnsense:
    baseurl: "https://some.domain.fqdn"
    creds: API_KEY_HERE:API_SECRET_HERE
`

const testYamlBadURL string = `---
opnsense:
    baseurl: "some.domain.fqdn"
    creds: API_KEY_HERE:API_SECRET_HERE
`

const testYamlBadAPI string = `---
opnsense:
    baseurl: "http://some.domain.fqdn"
    creds: invalid_creds
`

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		want    Config
		wantErr bool
	}{
		{
			name:    "missing file",
			path:    "nothing_here.yml",
			wantErr: true,
		},
		{
			name: "With file",
			path: func() string {
				cfgPath := path.Join(t.TempDir(), "config.yml")
				require.NoError(t, os.WriteFile(cfgPath, []byte(testYaml), 0600))
				return cfgPath
			}(),
			want: Config{
				Opnsense: Opnsense{
					BaseURL: "https://some.domain.fqdn",
					Creds:   "API_KEY_HERE:API_SECRET_HERE",
				},
				Listen: Listen{
					Addr: ":8080",
				},
			},
		},
		{
			name: "invalid baseurl",
			path: func() string {
				cfgPath := path.Join(t.TempDir(), "config.yml")
				require.NoError(t, os.WriteFile(cfgPath, []byte(testYamlBadURL), 0600))
				return cfgPath
			}(),
			want: Config{
				Opnsense: Opnsense{
					BaseURL: "some.domain.fqdn", // <-- problem
					Creds:   "API_KEY_HERE:API_SECRET_HERE",
				},
				Listen: Listen{
					Addr: ":8080",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid creds",
			path: func() string {
				cfgPath := path.Join(t.TempDir(), "config.yml")
				require.NoError(t, os.WriteFile(cfgPath, []byte(testYamlBadAPI), 0600))
				return cfgPath
			}(),
			want: Config{
				Opnsense: Opnsense{
					BaseURL: "http://some.domain.fqdn",
					Creds:   "invalid_creds",
				},
				Listen: Listen{
					Addr: ":8080",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Load(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
