package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/jferrl/go-githubauth"
	"golang.org/x/oauth2"
)

// NewInstallationHTTPClient creates an *http.Client that authenticates all
// requests as a GitHub App installation using the given private key PEM file.
func NewInstallationHTTPClient(appID, installationID int64, privateKeyPath string) (*http.Client, error) {
	pemBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	appTokenSource, err := githubauth.NewApplicationTokenSource(appID, pemBytes)
	if err != nil {
		return nil, fmt.Errorf("create app token source: %w", err)
	}

	installTokenSource := githubauth.NewInstallationTokenSource(installationID, appTokenSource)

	return oauth2.NewClient(context.Background(), installTokenSource), nil
}
