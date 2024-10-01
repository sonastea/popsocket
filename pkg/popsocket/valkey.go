package popsocket

import (
	"os"
	"strings"

	"github.com/valkey-io/valkey-go"
)

// loadValkeyInitAddress loads a []string from the REDIS_URL env variable and provides the
// default of []string{"127.0.0.1:6379"} when nothing is set.
func loadValkeyInitAddress() []string {
	addr := os.Getenv("REDIS_URL")
	if addr == "" {
		return []string{"127.0.0.1:6379"}
	}

	addrs := strings.Split(addr, ",")

	for i := range addrs {
		addrs[i] = strings.TrimSpace(addrs[i])
	}

	return addrs
}

// NewValkeyClient returns an instance of ValkeyClient with the provided options.
func NewValkeyClient(opts ...valkey.ClientOption) (valkey.Client, error) {
	addr := loadValkeyInitAddress()

	options := valkey.ClientOption{
		InitAddress: addr,
	}

	for _, opt := range opts {
		if opt.DisableCache {
			options.DisableCache = true
		}
	}

	client, err := valkey.NewClient(options)
	if err != nil {
		return nil, err
	}

	return client, nil
}
