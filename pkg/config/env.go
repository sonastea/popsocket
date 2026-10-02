package config

import (
	"fmt"
	"os"
	"reflect"
)

type env struct {
	DATABASE_URL struct {
		Value string `json:"value"`
	}
	SESSION_SECRET_KEY struct {
		Value string `json:"value"`
	}
}

var ENV env

// LoadEnvVars populates the Env object with the required environment variables.
// Unset or empty variables return an error without replacing previously loaded values.
func LoadEnvVars() error {
	var loaded env
	val := reflect.ValueOf(&loaded).Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		typeField := val.Type().Field(i)

		envValue := os.Getenv(typeField.Name)
		if envValue == "" {
			return fmt.Errorf("environment variable %s is required", typeField.Name)
		}

		valueField := field.FieldByName("Value")
		if valueField.IsValid() && valueField.CanSet() {
			valueField.SetString(envValue)
		}
	}
	ENV = loaded
	return nil
}
