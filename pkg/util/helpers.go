package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// UrlToStandardBase64 converts URL-safe Base64 characters to standard Base64.
func UrlToStandardBase64(s string) string {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	return s
}

// CalculateHMAC calculates the expected HMAC-SHA256 signature for a given value, secret,
// and signature. Comparing the expected signature to the given signature
// and returning the outcome as a bool.
func CalculateHMAC(value, secret, sig string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	expected = UrlToStandardBase64(expected)
	sig = UrlToStandardBase64(sig)

	if hmac.Equal([]byte(expected), []byte(sig)) {
		return true
	}

	return false
}

// DecodeCookie decodes the URL-encoded cookie value and verifies its HMAC signature.
// Returns sid if the cookie was verified and errors if not.
func DecodeCookie(str string, secret string) (string, error) {
	decodedValue, err := url.QueryUnescape(str)
	if err != nil {
		return "", fmt.Errorf("Unable to URL-decode cookie value: %w", err)
	}

	if !strings.HasPrefix(decodedValue, "s:") {
		return decodedValue, nil
	}

	signedValue := decodedValue[2:]

	parts := strings.SplitN(signedValue, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("Invalid signed cookie format")
	}

	originalSID := parts[0]
	signature := parts[1]

	if ok := CalculateHMAC(originalSID, secret, signature); !ok {
		return "", fmt.Errorf("Unable to calculate HMAC: %w", err)
	}

	return originalSID, nil
}
