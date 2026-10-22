package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMaxDecompressedBodySize(t *testing.T) {
	for _, tt := range []struct {
		name      string
		envValue  string
		want      int64
		wantError bool
	}{
		{name: "default", want: 64 << 20},
		{name: "environment", envValue: "167772160", want: 160 << 20},
		{name: "zero", envValue: "0", wantError: true},
		{name: "negative", envValue: "-1", wantError: true},
		{name: "invalid", envValue: "160MiB", wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_MAX_DECOMPRESSED_BODY_SIZE", tt.envValue)
			cfg, err := Load()
			if tt.wantError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "max_decompressed_body_size")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, cfg.Gateway.MaxDecompressedBodySize)
		})
	}
}
