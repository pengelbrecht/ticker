package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_IsEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "nil config defaults to enabled",
			config: nil,
			want:   true,
		},
		{
			name:   "nil Enabled field defaults to enabled",
			config: &Config{Enabled: nil},
			want:   true,
		},
		{
			name:   "explicitly enabled",
			config: &Config{Enabled: &trueVal},
			want:   true,
		},
		{
			name:   "explicitly disabled",
			config: &Config{Enabled: &falseVal},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.IsEnabled()
			if got != tt.want {
				t.Errorf("Config.IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		configJSON  string
		createFile  bool
		wantEnabled bool
		wantNil     bool
		wantErr     bool
	}{
		{
			name:       "missing file returns nil config",
			createFile: false,
			wantNil:    true,
			wantErr:    false,
		},
		{
			name:        "enabled true",
			configJSON:  `{"verification": {"enabled": true}}`,
			createFile:  true,
			wantEnabled: true,
			wantNil:     false,
			wantErr:     false,
		},
		{
			name:        "enabled false",
			configJSON:  `{"verification": {"enabled": false}}`,
			createFile:  true,
			wantEnabled: false,
			wantNil:     false,
			wantErr:     false,
		},
		{
			name:       "empty verification section defaults to enabled",
			configJSON: `{"verification": {}}`,
			createFile: true,
			wantNil:    false,
			wantErr:    false,
			// Config exists but Enabled is nil, so IsEnabled() returns true
			wantEnabled: true,
		},
		{
			name:       "missing verification section returns nil config",
			configJSON: `{}`,
			createFile: true,
			wantNil:    true,
			wantErr:    false,
		},
		{
			name:       "malformed JSON returns error",
			configJSON: `{"verification": {invalid json`,
			createFile: true,
			wantErr:    true,
		},
		{
			name:       "empty file returns error",
			configJSON: ``,
			createFile: true,
			wantErr:    true, // empty string is not valid JSON
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()

			if tt.createFile {
				// Create .ticker directory and config.json
				tickerDir := filepath.Join(tmpDir, ".ticker")
				if err := os.MkdirAll(tickerDir, 0755); err != nil {
					t.Fatalf("failed to create .ticker dir: %v", err)
				}
				configPath := filepath.Join(tickerDir, "config.json")
				if err := os.WriteFile(configPath, []byte(tt.configJSON), 0644); err != nil {
					t.Fatalf("failed to write config.json: %v", err)
				}
			}

			got, err := LoadConfig(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Error("LoadConfig() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadConfig() unexpected error: %v", err)
				return
			}

			if tt.wantNil {
				if got != nil {
					t.Errorf("LoadConfig() = %+v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Error("LoadConfig() = nil, want non-nil")
				return
			}

			if got.IsEnabled() != tt.wantEnabled {
				t.Errorf("LoadConfig().IsEnabled() = %v, want %v", got.IsEnabled(), tt.wantEnabled)
			}
		})
	}
}

func TestLoadConfig_ReadError(t *testing.T) {
	// Test that non-existent directory doesn't cause a crash
	// (should return nil, nil since file doesn't exist)
	config, err := LoadConfig("/nonexistent/directory/path")
	if err != nil {
		t.Errorf("LoadConfig() on non-existent dir returned error: %v", err)
	}
	if config != nil {
		t.Errorf("LoadConfig() on non-existent dir = %+v, want nil", config)
	}
}
