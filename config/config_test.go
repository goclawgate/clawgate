package config

import (
	"os"
	"testing"
)

func ptrBool(b bool) *bool { return &b }

// TestFastModePrecedence exercises the env/flag/nil permutations for
// FAST_MODE through config.Load, proving the layering works:
//
//	flag (non-nil) > env > default (false)
func TestFastModePrecedence(t *testing.T) {
	// Isolate from any .env in the repo root.
	orig, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	tests := []struct {
		name    string
		envVal  string // "" means unset
		flag    *bool  // nil means flag not passed
		want    bool
	}{
		{
			name:   "env wins when flag is nil",
			envVal: "1",
			flag:   nil,
			want:   true,
		},
		{
			name:   "flag true overrides env unset",
			envVal: "",
			flag:   ptrBool(true),
			want:   true,
		},
		{
			name:   "flag false overrides env true",
			envVal: "1",
			flag:   ptrBool(false),
			want:   false,
		},
		{
			name:   "neither set defaults to false",
			envVal: "",
			flag:   nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("FAST_MODE", tt.envVal)
			} else {
				t.Setenv("FAST_MODE", "")
			}

			cfg := Load(FlagOverrides{FastMode: tt.flag})
			if cfg.FastMode != tt.want {
				t.Errorf("FastMode = %v, want %v", cfg.FastMode, tt.want)
			}
		})
	}
}
