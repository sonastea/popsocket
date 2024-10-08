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

// LoadEnvVars populates the Env object with the required
// environment variables to be utilized in PopSocket.
func LoadEnvVars() {
	val := reflect.ValueOf(&ENV).Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		typeField := val.Type().Field(i)

		envValue, exists := os.LookupEnv(typeField.Name)
		if !exists {
			panic(fmt.Sprintf("Environment variable %s is required", typeField.Name))
		}

		valueField := field.FieldByName("Value")
		if valueField.IsValid() && valueField.CanSet() {
			valueField.SetString(envValue)
		}
	}
}
