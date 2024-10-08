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

// CalculateHMAC calculates the HMAC-SHA256 signature for a given value and secret key.
func CalculateHMAC(value, secret string) (string, error) {
	h := hmac.New(sha256.New, []byte(secret))
	_, err := h.Write([]byte(value))
	if err != nil {
		return "", fmt.Errorf("Unable to write HMAC: %w", err)
	}

	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	return signature, nil
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

	originalValue := parts[0]
	signature := parts[1]

	expected, err := CalculateHMAC(originalValue, secret)
	if err != nil {
		return "", fmt.Errorf("Unable to calculate HMAC: %w", err)
	}

	expected = UrlToStandardBase64(expected)
	signature = UrlToStandardBase64(signature)

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return "", fmt.Errorf("Expected signature %v, got %v", expected, signature)
	}

	return originalValue, nil
}
