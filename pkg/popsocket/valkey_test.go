package popsocket

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// TestLoadValkeyInitAddress_WithValue tests that the function correctly
// loads a single address from the REDIS_URL environment variable.
func TestLoadValkeyInitAddress_WithValue(t *testing.T) {
	t.Parallel()

	expected := "test.local:6379"
	os.Setenv("REDIS_URL", expected)

	addr := loadValkeyInitAddress()

	if addr[0] != expected {
		t.Fatalf("Addr with value should be `%v`, but got %v", expected, addr)
	}
}

// TestLoadValkeyInitAddress_WithMultipleValues tests that the function handles
// multiple comma-separated addresses and trims spaces around each address.
func TestLoadValkeyInitAddress_WithMultipleValues(t *testing.T) {
	t.Parallel()

	initialAddrs := []string{"redis.local:6379 ", " test.local:6379", " redis2.local:6379 "}
	os.Setenv("REDIS_URL", strings.Join(initialAddrs, ","))

	expected := make([]string, 3)
	for i := range initialAddrs {
		expected[i] = strings.TrimSpace(initialAddrs[i])
	}
	addr := loadValkeyInitAddress()

	if !reflect.DeepEqual(addr, expected) {
		t.Fatalf("Valkey init addresses should be `%v`, but got %v", expected, addr)
	}
}

// TestLoadValkeyInitAddress_WithoutValue tests that the function returns the
// default address when the REDIS_URL environment variable is not set.
func TestLoadValkeyInitAddress_WithoutValue(t *testing.T) {
	t.Parallel()

	os.Unsetenv("REDIS_URL")

	addr := loadValkeyInitAddress()
	expected := "127.0.0.1:6379"

	if addr[0] != expected {
		t.Fatalf("Addr without value should be `%v`, but got %v", expected, addr)
	}
}

// TestNewValkeyService tests that newValkeyService successfully creates
// a ValkeyService instance when a valid Redis server is available.
func TestNewValkeyService(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	_, err := newValkeyService()
	if err != nil {
		t.Fatalf("New valkey service failed %v", err)
	}
}

// TestNewValkeyService_ExpectFail test that newValkeyService returns
// an error when it cannot connect to the specified Redis server.
func TestNewValkeyService_ExpectFail(t *testing.T) {
	os.Setenv("REDIS_URL", "127.0.0.1:6380")

	_, err := newValkeyService()
	if err == nil {
		t.Fatalf("New valkey service expected to fail: %v \n", err)
	}
}
