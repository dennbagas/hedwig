package config

import "testing"

func TestValidateRepoCallbackDataLen(t *testing.T) {
	if err := validateRepoCallbackDataLen([]RepoConfig{
		{Owner: "my-org", Name: "my-service"},
	}); err != nil {
		t.Errorf("expected no error for a normal repo, got %v", err)
	}

	longName := "a-very-long-repository-name-that-pushes-callback-data-over-the-limit"
	if err := validateRepoCallbackDataLen([]RepoConfig{
		{Owner: "some-organization", Name: longName},
	}); err == nil {
		t.Error("expected an error for a repo whose callback data exceeds Telegram's limit, got nil")
	}
}
