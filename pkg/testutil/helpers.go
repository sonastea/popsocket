package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"

	"github.com/sonastea/popsocket/pkg/config"
	"github.com/sonastea/popsocket/pkg/util"
)

// createRandomSecretKey generates a random session secret key for testing.
func createRandomSecretKey() (string, error) {
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("Unable to generate random secret key: %w", err)
	}
	return hex.EncodeToString(randomBytes), nil
}

// CreateSignedCookie generates a signed connect.sid cookie for testing.
func CreateSignedCookie(sid, secretKey string) (string, error) {
	signature, err := util.CalculateHMAC(sid, secretKey)
	if err != nil {
		return "", err
	}

	signedValue := fmt.Sprintf("s:%s.%s", sid, signature)

	return url.QueryEscape(signedValue), nil
}

// SetRandomTestSecretKey sets a random session secret key in the environment for testing.
func SetRandomTestSecretKey() (string, error) {
	secretKey, err := createRandomSecretKey()
	if err != nil {
		return "", err
	}
	os.Setenv("SESSION_SECRET_KEY", secretKey)
	// we have to reload environment variables into the config package
	config.LoadEnvVars()
	return secretKey, nil
}
