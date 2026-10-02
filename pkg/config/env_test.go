package config

import (
	"os"
	"strings"
	"testing"
)

type testcase struct {
	envVars             map[string]string
	name                string
	expectedDatabaseURL string
	missingVariable     string
}

// TestLoadEnvVars ensures required variables are loaded and missing or empty
// variables return an error.
func TestLoadEnvVars(t *testing.T) {
	original := ENV
	t.Cleanup(func() { ENV = original })

	tests := []testcase{
		{
			name: "All Environment Variables Set",
			envVars: map[string]string{
				"DATABASE_URL":       "postgresql://localhost:5432/db",
				"SESSION_SECRET_KEY": "test-key",
			},
			expectedDatabaseURL: "postgresql://localhost:5432/db",
		},
		{
			name:            "Missing Database URL",
			envVars:         map[string]string{"SESSION_SECRET_KEY": "test-key"},
			missingVariable: "DATABASE_URL",
		},
		{
			name:            "Empty Database URL",
			envVars:         map[string]string{"DATABASE_URL": "", "SESSION_SECRET_KEY": "test-key"},
			missingVariable: "DATABASE_URL",
		},
		{
			name:            "Missing Environment Variables",
			envVars:         map[string]string{},
			missingVariable: "DATABASE_URL",
		},
		{
			name:            "Missing Session Secret Key",
			envVars:         map[string]string{"DATABASE_URL": "postgresql://localhost:5432/db"},
			missingVariable: "SESSION_SECRET_KEY",
		},
		{
			name:            "Empty Session Secret Key",
			envVars:         map[string]string{"DATABASE_URL": "postgresql://localhost:5432/db", "SESSION_SECRET_KEY": ""},
			missingVariable: "SESSION_SECRET_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"DATABASE_URL", "SESSION_SECRET_KEY"} {
				value, exists := tt.envVars[key]
				t.Setenv(key, value)
				if !exists {
					if err := os.Unsetenv(key); err != nil {
						t.Fatal(err)
					}
				}
			}

			previous := ENV
			err := LoadEnvVars()
			if tt.missingVariable != "" {
				if err == nil || !strings.Contains(err.Error(), tt.missingVariable) {
					t.Fatalf("Expected missing %s error, got %v", tt.missingVariable, err)
				}
				if ENV != previous {
					t.Fatal("Invalid configuration replaced previously loaded values")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if ENV.DATABASE_URL.Value != tt.expectedDatabaseURL {
				t.Errorf("Expected DATABASE_URL %q, got %q", tt.expectedDatabaseURL, ENV.DATABASE_URL.Value)
			}

			wantSecret, ok := tt.envVars["SESSION_SECRET_KEY"]
			if !ok || ENV.SESSION_SECRET_KEY.Value != wantSecret {
				t.Errorf("Expected SESSION_SECRET_KEY %q, got %q", wantSecret, ENV.SESSION_SECRET_KEY.Value)
			}
		})
	}
}
