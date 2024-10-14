package testutil

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(sid))

	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	signature = util.UrlToStandardBase64(signature)

	signedValue := fmt.Sprintf("s:%s.%s", sid, signature)
	return url.QueryEscape(signedValue), nil
}

func UrlToStandardBase64(signature string) {
	panic("unimplemented")
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
