package cmd

import (
	"bytes"
	"io"
	"path"
	"testing"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/stretchr/testify/assert"
)

func Test_getCfgPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reader  io.Reader
		want    string
		wantErr error
	}{
		{
			name:   "uses default",
			reader: &bytes.Buffer{},
			want:   cfgFile,
		},
		{
			name:   "valid input",
			reader: bytes.NewBufferString("/some/path/here.yaml\n"),
			want:   "/some/path/here.yaml",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := getCfgPath(tt.reader)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func Test_getBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reader  io.Reader
		want    string
		wantErr error
	}{
		{
			name:    "no input",
			reader:  &bytes.Buffer{},
			want:    "",
			wantErr: ErrRequired,
		},
		{
			name:   "valid input",
			reader: bytes.NewBufferString("https://some.url.here\n"),
			want:   "https://some.url.here",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := readBaseURL(tt.reader)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func Test_getOPNSenseSecret(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reader  io.Reader
		want    string
		wantErr error
	}{
		{
			name:    "no input",
			reader:  &bytes.Buffer{},
			want:    "",
			wantErr: ErrRequired,
		},
		{
			name:   "valid input",
			reader: bytes.NewBufferString("key:secret\n"),
			want:   "key:secret",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := readOPNSenseSecret(tt.reader)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func Test_getCfg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reader  io.Reader
		want    config.Config
		wantErr error
	}{
		{
			name:    "no input",
			reader:  &bytes.Buffer{},
			want:    config.Config{},
			wantErr: ErrRequired,
		},
		{
			name: "valid input",
			reader: io.MultiReader(
				bytes.NewBufferString("https://some.url.here\n"),
				bytes.NewBufferString("key:secret\n"),
			),
			want: config.Config{
				Opnsense: config.Opnsense{
					BaseURL: "https://some.url.here",
					Creds:   "key:secret",
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := getCfg(tt.reader)
			assert.Equal(t, tt.want, got)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func Test_writeCfg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		cfgFilePath string
		cfg         config.Config
		wantErr     bool
	}{
		{
			name:        "simple test",
			cfgFilePath: path.Join(t.TempDir(), "must_be_created", "unbound.yaml"),
			cfg: config.Config{
				Opnsense: config.Opnsense{
					BaseURL: "https://some.url.here",
					Creds:   "key:secret",
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, writeCfg(tt.cfgFilePath, tt.cfg))
			foundCfg, err := config.Load(tt.cfgFilePath)
			assert.NoError(t, err)
			assert.Equal(t, tt.cfg.Opnsense, foundCfg.Opnsense)
		})
	}
}

func Test_readWriteConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reader  io.Reader
		wantErr error
	}{
		{
			name: "Simple full test",
			reader: io.MultiReader(
				bytes.NewBufferString(path.Join(t.TempDir(), "unbound.yml")+"\n"),
				bytes.NewBufferString("https://some.url.here\n"),
				bytes.NewBufferString("key:secret\n"),
			),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.ErrorIs(t, readWriteConfig(tt.reader), tt.wantErr)
		})
	}
}
