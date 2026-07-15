package githubapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v88/github"
)

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newSignedWebhookRequest(secret string, body []byte, signature string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	return req
}

func TestValidateWebhookValidSignature(t *testing.T) {
	c, _ := New(http.DefaultClient, "my-secret")
	body := []byte(`{"zen":"hello"}`)
	req := newSignedWebhookRequest("my-secret", body, signBody("my-secret", body))

	payload, err := c.ValidateWebhook(req)
	if err != nil {
		t.Fatalf("ValidateWebhook() error = %v", err)
	}
	if string(payload) != string(body) {
		t.Errorf("ValidateWebhook() payload = %q, want %q", payload, body)
	}
}

func TestValidateWebhookWrongSecret(t *testing.T) {
	c, _ := New(http.DefaultClient, "my-secret")
	body := []byte(`{"zen":"hello"}`)
	req := newSignedWebhookRequest("my-secret", body, signBody("wrong-secret", body))

	_, err := c.ValidateWebhook(req)
	if err == nil {
		t.Fatal("ValidateWebhook() error = nil, want error for a signature computed with the wrong secret")
	}
}

func TestValidateWebhookTamperedBody(t *testing.T) {
	c, _ := New(http.DefaultClient, "my-secret")
	signedBody := []byte(`{"zen":"hello"}`)
	sig := signBody("my-secret", signedBody)
	// Send a different body than the one the signature was computed over.
	req := newSignedWebhookRequest("my-secret", []byte(`{"zen":"tampered"}`), sig)

	_, err := c.ValidateWebhook(req)
	if err == nil {
		t.Fatal("ValidateWebhook() error = nil, want error when body doesn't match the signature")
	}
}

func TestValidateWebhookMissingSignature(t *testing.T) {
	c, _ := New(http.DefaultClient, "my-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")

	_, err := c.ValidateWebhook(req)
	if err == nil {
		t.Fatal("ValidateWebhook() error = nil, want error for a request with no signature header")
	}
}

func TestParseWebhookKnownEventType(t *testing.T) {
	c, _ := New(http.DefaultClient, "secret")
	payload := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"acme/widgets"}}`)

	event, err := c.ParseWebhook("push", payload)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	pushEvent, ok := event.(*github.PushEvent)
	if !ok {
		t.Fatalf("ParseWebhook() returned %T, want *github.PushEvent", event)
	}
	if pushEvent.GetRef() != "refs/heads/main" {
		t.Errorf("PushEvent.GetRef() = %q, want refs/heads/main", pushEvent.GetRef())
	}
}

func TestParseWebhookUnknownEventType(t *testing.T) {
	c, _ := New(http.DefaultClient, "secret")
	_, err := c.ParseWebhook("not_a_real_event_type", []byte(`{}`))
	if err == nil {
		t.Fatal("ParseWebhook() error = nil, want error for an unrecognized event type")
	}
}

// newTestGithubClient starts a fake GitHub API server driven by handler and
// returns a githubClient pointed at it.
func newTestGithubClient(t *testing.T, handler http.HandlerFunc) *githubClient {
	t.Helper()
	server := httptest.NewServer(http.StripPrefix("/api/v3", handler))
	t.Cleanup(server.Close)

	gh, err := github.NewClient(
		github.WithHTTPClient(server.Client()),
		github.WithEnterpriseURLs(server.URL+"/", server.URL+"/"),
	)
	if err != nil {
		t.Fatalf("create github client: %v", err)
	}

	return &githubClient{client: gh, webhookSecret: "unused"}
}

func TestRerunFailedJobsSuccess(t *testing.T) {
	var gotPath, gotMethod string
	c := newTestGithubClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusCreated)
	})

	err := c.RerunFailedJobs(context.Background(), "acme", "widgets", 42)
	if err != nil {
		t.Fatalf("RerunFailedJobs() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/repos/acme/widgets/actions/runs/42/rerun-failed-jobs" {
		t.Errorf("path = %q, want /repos/acme/widgets/actions/runs/42/rerun-failed-jobs", gotPath)
	}
}

func TestRerunFailedJobsAPIError(t *testing.T) {
	c := newTestGithubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"message":"run is already in progress"}`)
	})

	err := c.RerunFailedJobs(context.Background(), "acme", "widgets", 42)
	if err == nil {
		t.Fatal("RerunFailedJobs() error = nil, want the API error to propagate")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("RerunFailedJobs() error = %q, want it to include the API message", err.Error())
	}
}

func TestCreatePRSuccess(t *testing.T) {
	c := newTestGithubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/pulls" {
			t.Errorf("path = %q, want /repos/acme/widgets/pulls", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"html_url":"https://github.com/acme/widgets/pull/7"}`)
	})

	url, err := c.CreatePR(context.Background(), "acme", "widgets", "My title", "My body", "feature", "main")
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}
	if url != "https://github.com/acme/widgets/pull/7" {
		t.Errorf("CreatePR() = %q, want the PR's HTML URL", url)
	}
}

func TestCreatePRNotFound(t *testing.T) {
	c := newTestGithubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	})

	_, err := c.CreatePR(context.Background(), "acme", "widgets", "t", "b", "feature", "main")
	if err == nil {
		t.Fatal("CreatePR() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not found or no access") {
		t.Errorf("CreatePR() error = %q, want the friendly not-found message", err.Error())
	}
}

func TestCreatePRUnprocessableHeadBranch(t *testing.T) {
	c := newTestGithubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"message":"Validation Failed","errors":[{"resource":"PullRequest","field":"head","code":"invalid"}]}`)
	})

	_, err := c.CreatePR(context.Background(), "acme", "widgets", "t", "b", "does-not-exist", "main")
	if err == nil {
		t.Fatal("CreatePR() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `source branch "does-not-exist" not found`) {
		t.Errorf("CreatePR() error = %q, want it to name the missing source branch", err.Error())
	}
}

func TestCreatePRUnprocessableBaseBranch(t *testing.T) {
	c := newTestGithubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"message":"Validation Failed","errors":[{"resource":"PullRequest","field":"base","code":"invalid"}]}`)
	})

	_, err := c.CreatePR(context.Background(), "acme", "widgets", "t", "b", "feature", "does-not-exist")
	if err == nil {
		t.Fatal("CreatePR() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `target branch "does-not-exist" not found`) {
		t.Errorf("CreatePR() error = %q, want it to name the missing target branch", err.Error())
	}
}

func TestCreatePRUnprocessableCustomMessage(t *testing.T) {
	c := newTestGithubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"message":"Validation Failed","errors":[{"resource":"PullRequest","code":"custom","message":"A pull request already exists for acme:feature."}]}`)
	})

	_, err := c.CreatePR(context.Background(), "acme", "widgets", "t", "b", "feature", "main")
	if err == nil {
		t.Fatal("CreatePR() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "A pull request already exists") {
		t.Errorf("CreatePR() error = %q, want the custom GitHub message to surface", err.Error())
	}
}
