package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"hedwig/internal/database"
	"hedwig/internal/logging"
	"hedwig/internal/retry"
	"hedwig/internal/telegrambot"
)

// slackSignatureMaxAge rejects Slack interaction requests whose timestamp is
// further from "now" than this, guarding against replay attacks — mirrors
// Slack's own documented guidance for verifying requests.
const slackSignatureMaxAge = 5 * time.Minute

// slackBlockActionsPayload is the minimal subset of Slack's block_actions
// interaction payload Hedwig needs: https://api.slack.com/reference/interaction-payloads/block-actions
type slackBlockActionsPayload struct {
	Type    string `json:"type"`
	Actions []struct {
		Value string `json:"value"`
	} `json:"actions"`
	Channel struct {
		ID string `json:"id"`
	} `json:"channel"`
	Message struct {
		Ts string `json:"ts"`
	} `json:"message"`
}

func (s *Server) handleSlackWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error().Err(err).Msg("read slack webhook body")
		w.WriteHeader(http.StatusOK)
		return
	}

	if !s.verifySlackSignature(r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), body) {
		logger.Warn().Msg("invalid slack signature")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		logger.Warn().Err(err).Msg("parse slack webhook body")
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload slackBlockActionsPayload
	if err := json.Unmarshal([]byte(values.Get("payload")), &payload); err != nil {
		logger.Warn().Err(err).Msg("unmarshal slack interaction payload")
		w.WriteHeader(http.StatusOK)
		return
	}

	if payload.Type != "block_actions" || len(payload.Actions) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	feature, _, retryPayload, err := telegrambot.DecodeCallback(payload.Actions[0].Value)
	if err != nil {
		logger.Warn().Err(err).Str("value", payload.Actions[0].Value).Msg("decode slack callback value")
		w.WriteHeader(http.StatusOK)
		return
	}

	switch feature {
	case retry.CallbackFeature:
		retryID, err := strconv.ParseInt(retryPayload, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		// callbackQueryID is empty: Slack has no separate "answer this
		// callback" API call the way Telegram does — the interaction is
		// acknowledged simply by returning 200 here.
		if err := s.retryH.HandleCallback(ctx, "", database.PlatformSlack, payload.Channel.ID, payload.Message.Ts, retryID); err != nil {
			logger.Error().Err(err).Msg("handle slack callback")
		}
	default:
		logger.Warn().Str("feature", feature).Msg("unknown slack callback feature")
	}

	w.WriteHeader(http.StatusOK)
}

// verifySlackSignature checks the v0 HMAC-SHA256 signature Slack attaches to
// every interaction request: https://docs.slack.dev/authentication/verifying-requests-from-slack
func (s *Server) verifySlackSignature(timestampHeader, signatureHeader string, body []byte) bool {
	if timestampHeader == "" || signatureHeader == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return false
	}
	age := time.Since(time.Unix(ts, 0))
	if age < 0 {
		age = -age
	}
	if age > slackSignatureMaxAge {
		return false
	}

	mac := hmac.New(sha256.New, []byte(s.slackSigningSecret))
	mac.Write([]byte("v0:" + timestampHeader + ":"))
	mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signatureHeader))
}
