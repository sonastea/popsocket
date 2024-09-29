package popsocket

import (
	"os"
	"strings"

	"github.com/valkey-io/valkey-go"
)

type ValkeyService struct {
	client *valkey.Client
}

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

func newValkeyService(opts ...valkey.ClientOption) (*ValkeyService, error) {
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

	return &ValkeyService{
		&client,
	}, nil
}
