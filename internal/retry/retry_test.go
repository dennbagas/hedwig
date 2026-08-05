package retry

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hedwig/internal/database"
	"hedwig/internal/githubapp/githubapptest"
	"hedwig/internal/slackbot/slackbottest"
	"hedwig/internal/telegrambot"
	"hedwig/internal/telegrambot/telegrambottest"

	"github.com/rs/zerolog"
)

const testSlackChannel = "C1"

// newTestRepo returns a real SQLite-backed repository (temp file, cleaned up
// automatically) plus the underlying *sql.DB for tests that need to
// manipulate rows directly (e.g. backdating timestamps for expiry tests).
func newTestRepo(t *testing.T) (*sql.DB, database.Repository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close() error = %v", err)
		}
	})
	return db, database.NewSQLiteRepository(db)
}

func newTestHandler(t *testing.T) (*Handler, *telegrambottest.FakeClient, *slackbottest.FakeClient, *githubapptest.FakeClient, database.Repository) {
	t.Helper()
	_, store := newTestRepo(t)
	return newTestHandlerWithStore(t, store)
}

// newTestHandlerWithStore is like newTestHandler but reuses an
// already-opened store, for tests (e.g. sweep_test.go) that need direct SQL
// access (backdating timestamps) alongside the Handler under test.
func newTestHandlerWithStore(t *testing.T, store database.Repository) (*Handler, *telegrambottest.FakeClient, *slackbottest.FakeClient, *githubapptest.FakeClient, database.Repository) {
	t.Helper()
	tg := telegrambottest.New()
	slack := slackbottest.New()
	gh := githubapptest.New()
	return New(store, tg, slack, testSlackChannel, gh, zerolog.Nop()), tg, slack, gh, store
}

const testTelegramText = `CI/CD failed: <b>Build</b>
acme/widgets
<a href="https://github.com/acme/widgets/actions/runs/55">View run</a>`

const testSlackText = "CI/CD failed: *Build*\nacme/widgets\n<https://github.com/acme/widgets/actions/runs/55|View run>"

func TestNotifyFailureSuccessBothPlatforms(t *testing.T) {
	h, tg, slack, _, store := newTestHandler(t)
	ctx := context.Background()

	err := h.NotifyFailure(ctx, 100, "Build", "acme", "widgets", 55, FailureText{Telegram: testTelegramText, Slack: testSlackText})
	if err != nil {
		t.Fatalf("NotifyFailure() error = %v", err)
	}

	if len(tg.Sent) != 2 {
		t.Fatalf("len(tg.Sent) = %d, want 2 (initial send + button edit)", len(tg.Sent))
	}
	sendMsg := tg.Sent[0]
	if sendMsg.Edited {
		t.Error("first telegram message should be a Send, not an Edit")
	}
	if sendMsg.Text != testTelegramText {
		t.Errorf("sent text = %q, want %q", sendMsg.Text, testTelegramText)
	}

	editMsg := tg.Sent[1]
	if !editMsg.Edited {
		t.Error("second telegram message should be an Edit (attaching the button)")
	}
	if len(editMsg.Params.Keyboard) != 1 || len(editMsg.Params.Keyboard[0]) != 1 {
		t.Fatalf("editMsg keyboard = %+v, want one row with one button", editMsg.Params.Keyboard)
	}
	btn := editMsg.Params.Keyboard[0][0]
	if btn.Text != "Retry failed jobs" {
		t.Errorf("telegram button text = %q, want %q", btn.Text, "Retry failed jobs")
	}

	feature, action, payload, err := telegrambot.DecodeCallback(btn.CallbackData)
	if err != nil {
		t.Fatalf("DecodeCallback(%q) error = %v", btn.CallbackData, err)
	}
	if feature != "retry" || action != "trigger" {
		t.Errorf("callback = (%q,%q), want (retry, trigger)", feature, action)
	}
	retryID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		t.Fatalf("callback payload %q is not an int: %v", payload, err)
	}

	if len(slack.Sent) != 1 {
		t.Fatalf("len(slack.Sent) = %d, want 1", len(slack.Sent))
	}
	slackMsg := slack.Sent[0]
	if slackMsg.Channel != testSlackChannel {
		t.Errorf("slack channel = %q, want %q", slackMsg.Channel, testSlackChannel)
	}
	if slackMsg.Text != testSlackText {
		t.Errorf("slack text = %q, want %q", slackMsg.Text, testSlackText)
	}
	if len(slackMsg.Buttons) != 1 || slackMsg.Buttons[0].Text != "Retry failed jobs" {
		t.Fatalf("slack buttons = %+v, want one Retry failed jobs button", slackMsg.Buttons)
	}

	rec, err := store.GetRetry(ctx, retryID)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec == nil {
		t.Fatal("expected a retry record to have been persisted")
	}
	if rec.RunID != 55 || rec.Repo != "acme/widgets" || rec.Status != database.RetryStatusPending {
		t.Errorf("persisted retry = %+v, want matching fields", rec)
	}

	targets, err := store.ListRetryTargets(ctx, retryID)
	if err != nil {
		t.Fatalf("ListRetryTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("ListRetryTargets() = %+v, want 2 targets (telegram + slack)", targets)
	}
	byPlatform := map[string]database.RetryTarget{}
	for _, tgt := range targets {
		byPlatform[tgt.Platform] = tgt
	}
	if tg := byPlatform[database.PlatformTelegram]; tg.ChatRef != "100" || tg.MessageText != testTelegramText {
		t.Errorf("telegram target = %+v, want ChatRef=100 MessageText=%q", tg, testTelegramText)
	}
	if sl := byPlatform[database.PlatformSlack]; sl.ChatRef != testSlackChannel || sl.MessageText != testSlackText {
		t.Errorf("slack target = %+v, want ChatRef=%s MessageText=%q", sl, testSlackChannel, testSlackText)
	}
}

func TestNotifyFailureTelegramOnly(t *testing.T) {
	h, tg, slack, _, _ := newTestHandler(t)
	ctx := context.Background()

	if err := h.NotifyFailure(ctx, 100, "Build", "acme", "widgets", 55, FailureText{Telegram: testTelegramText}); err != nil {
		t.Fatalf("NotifyFailure() error = %v", err)
	}
	if len(tg.Sent) != 2 {
		t.Errorf("len(tg.Sent) = %d, want 2", len(tg.Sent))
	}
	if len(slack.Sent) != 0 {
		t.Errorf("len(slack.Sent) = %d, want 0 (no slack text rendered)", len(slack.Sent))
	}
}

func TestNotifyFailureSendMessageError(t *testing.T) {
	h, tg, _, _, store := newTestHandler(t)
	tg.SendMessageErr = errors.New("telegram is down")

	err := h.NotifyFailure(context.Background(), 1, "Build", "acme", "widgets", 1, FailureText{Telegram: testTelegramText})
	if err != nil {
		t.Fatalf("NotifyFailure() error = %v, want nil — a single platform's send failure is logged, not fatal", err)
	}

	rec, _ := store.GetRetry(context.Background(), 1)
	if rec == nil {
		t.Error("expected the retry record to still be created even though the telegram send failed")
	}
}

func TestNotifyFailureButtonAttachErrorStillSucceeds(t *testing.T) {
	h, tg, _, _, store := newTestHandler(t)
	tg.EditMessageErr = errors.New("edit failed")

	err := h.NotifyFailure(context.Background(), 1, "Build", "acme", "widgets", 1, FailureText{Telegram: testTelegramText})
	if err != nil {
		t.Fatalf("NotifyFailure() error = %v, want nil even though attaching the button failed", err)
	}

	rec, err := store.GetRetry(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec == nil {
		t.Fatal("expected the retry record to exist even though the button never got attached")
	}
	if rec.Status != database.RetryStatusPending {
		t.Errorf("status = %q, want still pending", rec.Status)
	}
}

func TestHandleCallbackUnknownRetryID(t *testing.T) {
	h, tg, _, gh, _ := newTestHandler(t)

	if err := h.HandleCallback(context.Background(), "cbq-1", database.PlatformTelegram, "100", "200", 9999); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 0 {
		t.Errorf("RerunCalls = %+v, want none for an unknown retry ID", gh.RerunCalls)
	}
	if len(tg.Sent) != 1 || !strings.Contains(tg.Sent[0].Text, "no longer valid") {
		t.Fatalf("tg.Sent = %+v, want a single 'no longer valid' message", tg.Sent)
	}
	if tg.Sent[0].ChatID != 100 || tg.Sent[0].MessageID != 200 {
		t.Errorf("edited (chatID=%d, messageID=%d), want the tapped message (100, 200)", tg.Sent[0].ChatID, tg.Sent[0].MessageID)
	}
	if tg.Sent[0].Params.Keyboard == nil || len(tg.Sent[0].Params.Keyboard) != 0 {
		t.Errorf("keyboard = %+v, want empty keyboard to remove the button", tg.Sent[0].Params.Keyboard)
	}
	if len(tg.AnsweredCallbacks) != 1 || tg.AnsweredCallbacks[0].CallbackQueryID != "cbq-1" {
		t.Errorf("AnsweredCallbacks = %+v, want the callback query answered", tg.AnsweredCallbacks)
	}
}

func TestHandleCallbackNonPendingStatusFansOutToBothPlatforms(t *testing.T) {
	h, tg, slack, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 3, Repo: "a/b", Status: database.RetryStatusExpired})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "2", MessageText: testTelegramText}); err != nil {
		t.Fatalf("CreateRetryTarget(telegram) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformSlack, ChatRef: testSlackChannel, MessageRef: "999.111", MessageText: testSlackText}); err != nil {
		t.Fatalf("CreateRetryTarget(slack) error = %v", err)
	}

	if err := h.HandleCallback(ctx, "cbq-1", database.PlatformTelegram, "1", "2", id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 0 {
		t.Errorf("RerunCalls = %+v, want none for an already-expired retry", gh.RerunCalls)
	}
	if !strings.Contains(tg.Sent[0].Text, "no longer valid") {
		t.Errorf("telegram text = %q, want 'no longer valid'", tg.Sent[0].Text)
	}
	if tg.Sent[0].Params.Keyboard == nil || len(tg.Sent[0].Params.Keyboard) != 0 {
		t.Errorf("telegram keyboard = %+v, want empty keyboard to remove the button", tg.Sent[0].Params.Keyboard)
	}
	if len(slack.Sent) != 1 || !strings.Contains(slack.Sent[0].Text, "no longer valid") {
		t.Fatalf("slack.Sent = %+v, want a single 'no longer valid' message (tapping either platform updates both)", slack.Sent)
	}
	if len(slack.Sent[0].Buttons) != 0 {
		t.Errorf("slack buttons = %+v, want none (button cleared)", slack.Sent[0].Buttons)
	}
}

func TestHandleCallbackRerunError(t *testing.T) {
	h, tg, _, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 55, Repo: "acme/widgets", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "2", MessageText: testTelegramText}); err != nil {
		t.Fatalf("CreateRetryTarget() error = %v", err)
	}
	gh.RerunFailedJobsErr = errors.New("run already active")

	if err := h.HandleCallback(ctx, "cbq-1", database.PlatformTelegram, "1", "2", id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one call", gh.RerunCalls)
	}
	call := gh.RerunCalls[0]
	if call.Owner != "acme" || call.Repo != "widgets" || call.RunID != 55 {
		t.Errorf("RerunCalls[0] = %+v, want acme/widgets run 55", call)
	}

	text := tg.Sent[0].Text
	if !strings.Contains(text, "already active") {
		t.Errorf("text = %q, want it to include the GitHub error", text)
	}
	if !strings.Contains(text, "github.com/acme/widgets/actions/runs/55") {
		t.Errorf("text = %q, want a link back to the run", text)
	}
	// The button must still work (same callback value), since the rerun can be retried.
	if len(tg.Sent[0].Params.Keyboard) != 1 || tg.Sent[0].Params.Keyboard[0][0].CallbackData == "" {
		t.Errorf("keyboard = %+v, want the retry button re-attached with a working callback", tg.Sent[0].Params.Keyboard)
	}

	rec, err := store.GetRetry(ctx, id)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec.Status != database.RetryStatusPending {
		t.Errorf("status = %q, want to remain pending after a failed retry attempt", rec.Status)
	}
}

func TestHandleCallbackRerunErrorEscapesHTML(t *testing.T) {
	h, tg, _, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 1, Repo: "acme/widgets", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "2", MessageText: testTelegramText}); err != nil {
		t.Fatalf("CreateRetryTarget() error = %v", err)
	}
	gh.RerunFailedJobsErr = errors.New("<script>bad</script>")

	if err := h.HandleCallback(ctx, "cbq-1", database.PlatformTelegram, "1", "2", id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	text := tg.Sent[0].Text
	if strings.Contains(text, "<script>") {
		t.Errorf("text = %q, contains an unescaped error message", text)
	}
}

func TestHandleCallbackRerunErrorEscapesSlackMrkdwn(t *testing.T) {
	h, _, slack, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 1, Repo: "acme/widgets", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformSlack, ChatRef: testSlackChannel, MessageRef: "1.1", MessageText: testSlackText}); err != nil {
		t.Fatalf("CreateRetryTarget() error = %v", err)
	}
	gh.RerunFailedJobsErr = errors.New("run <failed> & stopped")

	if err := h.HandleCallback(ctx, "", database.PlatformSlack, testSlackChannel, "1.1", id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	text := slack.Sent[0].Text
	if strings.Contains(text, "<failed>") || strings.Contains(text, " & ") {
		t.Errorf("text = %q, contains unescaped mrkdwn special characters", text)
	}
	if !strings.Contains(text, "&lt;failed&gt;") || !strings.Contains(text, "&amp;") {
		t.Errorf("text = %q, want the error message escaped for Slack mrkdwn", text)
	}
	// The link built from checkURL must survive intact (not itself mangled by escaping).
	if !strings.Contains(text, "<https://github.com/acme/widgets/actions/runs/1|Check on GitHub>") {
		t.Errorf("text = %q, want the GitHub link unescaped", text)
	}
}

func TestHandleCallbackDoubleTapOnlyRerunsOnce(t *testing.T) {
	// Simulates two near-simultaneous taps (one per platform) on the same
	// retry: the first call's ClaimPendingRetry should win, the second
	// should see the retry as already claimed and not call GitHub again.
	h, tg, _, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 55, Repo: "acme/widgets", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "2", MessageText: testTelegramText}); err != nil {
		t.Fatalf("CreateRetryTarget(telegram) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformSlack, ChatRef: testSlackChannel, MessageRef: "999.111", MessageText: testSlackText}); err != nil {
		t.Fatalf("CreateRetryTarget(slack) error = %v", err)
	}

	if err := h.HandleCallback(ctx, "cbq-1", database.PlatformTelegram, "1", "2", id); err != nil {
		t.Fatalf("first HandleCallback() error = %v", err)
	}
	if err := h.HandleCallback(ctx, "", database.PlatformSlack, testSlackChannel, "999.111", id); err != nil {
		t.Fatalf("second HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one — the second tap must not trigger a second rerun", gh.RerunCalls)
	}
	if !strings.Contains(tg.Sent[len(tg.Sent)-1].Text, "no longer valid") {
		t.Errorf("last telegram message should not be the second tap's own success text, want 'no longer valid'; got %q", tg.Sent[len(tg.Sent)-1].Text)
	}
}

func TestHandleCallbackSuccess(t *testing.T) {
	h, tg, slack, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, database.CICDRetry{RunID: 55, Repo: "acme/widgets", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "2", MessageText: testTelegramText}); err != nil {
		t.Fatalf("CreateRetryTarget(telegram) error = %v", err)
	}
	if err := store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformSlack, ChatRef: testSlackChannel, MessageRef: "999.111", MessageText: testSlackText}); err != nil {
		t.Fatalf("CreateRetryTarget(slack) error = %v", err)
	}

	if err := h.HandleCallback(ctx, "cbq-1", database.PlatformTelegram, "1", "2", id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one call", gh.RerunCalls)
	}
	if len(tg.Sent) != 1 || !tg.Sent[0].Edited {
		t.Fatalf("tg.Sent = %+v, want exactly one edited message", tg.Sent)
	}
	wantTelegramText := testTelegramText + "\n\n✅ Retry request sent"
	if tg.Sent[0].Text != wantTelegramText {
		t.Errorf("telegram text = %q, want %q", tg.Sent[0].Text, wantTelegramText)
	}
	if tg.Sent[0].Params.Keyboard == nil || len(tg.Sent[0].Params.Keyboard) != 0 {
		t.Errorf("telegram keyboard = %+v, want empty keyboard to remove the retry button", tg.Sent[0].Params.Keyboard)
	}

	wantSlackText := testSlackText + "\n\n✅ Retry request sent"
	if len(slack.Sent) != 1 || slack.Sent[0].Text != wantSlackText {
		t.Fatalf("slack.Sent = %+v, want one message with text %q (retry on telegram also updates slack)", slack.Sent, wantSlackText)
	}
	if len(slack.Sent[0].Buttons) != 0 {
		t.Errorf("slack buttons = %+v, want none (button cleared)", slack.Sent[0].Buttons)
	}

	rec, err := store.GetRetry(ctx, id)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec.Status != database.RetryStatusRetried {
		t.Errorf("status = %q, want %q", rec.Status, database.RetryStatusRetried)
	}
}

func TestSplitRepo(t *testing.T) {
	tests := []struct {
		in        string
		wantOwner string
		wantRepo  string
	}{
		{"acme/widgets", "acme", "widgets"},
		{"no-slash", "no-slash", ""},
		{"a/b/c", "a", "b/c"},
		{"", "", ""},
	}
	for _, tt := range tests {
		owner, repo := splitRepo(tt.in)
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("splitRepo(%q) = (%q, %q), want (%q, %q)", tt.in, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}
