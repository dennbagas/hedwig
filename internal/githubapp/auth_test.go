package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writeTestKey(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test key: %v", err)
	}
	return path
}

func TestNewInstallationHTTPClientNonexistentKeyPath(t *testing.T) {
	_, err := NewInstallationHTTPClient(1, 2, filepath.Join(t.TempDir(), "does-not-exist.pem"))
	if err == nil {
		t.Fatal("NewInstallationHTTPClient() error = nil, want error for a missing key file")
	}
}

func TestNewInstallationHTTPClientMalformedPEM(t *testing.T) {
	path := writeTestKey(t, []byte("not a pem file"))

	_, err := NewInstallationHTTPClient(1, 2, path)
	if err == nil {
		t.Fatal("NewInstallationHTTPClient() error = nil, want error for a malformed PEM file")
	}
}

func TestNewInstallationHTTPClientValidKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	path := writeTestKey(t, pemBytes)

	// The OAuth2 token exchange only happens lazily on the first real
	// request, so constructing the client with a structurally valid key
	// should succeed without making any network call.
	client, err := NewInstallationHTTPClient(1, 2, path)
	if err != nil {
		t.Fatalf("NewInstallationHTTPClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewInstallationHTTPClient() returned a nil client")
	}
}
