package config

import (
	"os"
	"reflect"
	"testing"
)

type testcase struct {
	name        string
	envVars     map[string]string
	shouldPanic bool
}

// TestLoadEnvVars ensures proper env vars are loaded and panics when not set.
func TestLoadEnvVars(t *testing.T) {
	tests := []testcase{
		{
			name: "All Environment Variables Set",
			envVars: map[string]string{
				"DATABASE_URL":       "postgresql://localhost:5432/db",
				"SESSION_SECRET_KEY": "test-key",
			},
			shouldPanic: false,
		},
		{
			name:        "Missing Environment Variables",
			envVars:     map[string]string{},
			shouldPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("DATABASE_URL")
			os.Unsetenv("SESSION_SECRET_KEY")

			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			didPanic := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						didPanic = true
					}
				}()
				LoadEnvVars()
			}()

			if tt.shouldPanic && !didPanic {
				t.Error("Expected panic but got none")
			} else if !tt.shouldPanic && didPanic {
				t.Error("Got unexpected panic")
			}

			if !didPanic && !tt.shouldPanic {
				for key, value := range tt.envVars {
					os.Setenv(key, value)
				}

				LoadEnvVars()
				val := reflect.ValueOf(&ENV).Elem()
				for i := 0; i < val.NumField(); i++ {
					typeField := val.Type().Field(i)
					envValue, exists := os.LookupEnv(typeField.Name)
					if !exists || envValue == "" {
						t.Fatalf("Expected environment variable %s to be set in test '%s', but it was not found", typeField.Name, tt.name)
					}
				}
			}
		})
	}
}
