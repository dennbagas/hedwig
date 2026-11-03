// Package approval computes a branch-aware view of a pull request's approval
// progress, for notification purposes only.
//
// Enforcement is left entirely to GitHub native branch protection / rulesets,
// which can require per-team approval counts ("Require review from specific
// teams" with a per-team Approvals value) and dismiss stale approvals on new
// commits. This package does not gate merges; it reads the current reviews,
// resolves each approver to the configured team roles, counts them against the
// rule for the PR's base branch, and returns a Result so the caller can post a
// progress message to Telegram/Slack.
//
// Two conventions, applied consistently:
//
//   - Multi-team approvers: an approver counts toward every configured role
//     they are an active member of. Someone on both developer-team and
//     infra-team therefore satisfies one slot of each quota at once.
//   - Stale approvals: this is stateless and always evaluates the reviews
//     GitHub reports at call time, taking the latest review per user. When
//     GitHub's "dismiss stale approvals" setting is on, dismissed reviews are
//     reported as DISMISSED and drop out of the count automatically. A
//     standing CHANGES_REQUESTED from any reviewer marks the PR unsatisfied.
package approval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"hedwig/internal/githubapp"

	"github.com/rs/zerolog"
)

// Config describes the approval rules mirrored from GitHub for messaging.
// Teams maps a role name (as used in Rules) to a GitHub team slug within Org.
// Rules maps a base branch name to the per-role minimum approval counts. A
// branch absent from Rules is not tracked — Evaluate returns no Result for it.
type Config struct {
	Enabled bool
	Org     string
	Teams   map[string]string
	Rules   map[string]map[string]int
}

// Handler evaluates approval progress for a pull request.
type Handler struct {
	gh     githubapp.Client
	cfg    Config
	logger zerolog.Logger
}

// New returns a Handler. When cfg.Enabled is false, Evaluate is a no-op.
func New(gh githubapp.Client, cfg Config, logger zerolog.Logger) *Handler {
	return &Handler{gh: gh, cfg: cfg, logger: logger}
}

// RoleStatus is one role's progress toward its required approval count.
type RoleStatus struct {
	Role string
	Have int
	Need int
}

// Met reports whether this role's quota is satisfied.
func (r RoleStatus) Met() bool { return r.Have >= r.Need }

// Result is the outcome of an approval evaluation, returned so the caller can
// render a status notification. Roles is sorted by role name.
type Result struct {
	Satisfied        bool
	Roles            []RoleStatus
	ChangesRequested []string // reviewer logins with a standing changes-request
}

// Enabled reports whether tracking is active (so callers can skip work early).
func (h *Handler) Enabled() bool { return h.cfg.Enabled }

// Evaluate counts the current approvals on the pull request and returns the
// per-role progress against the base branch's rule.
//
// Returns (nil, nil) when tracking is disabled or no rule exists for
// baseBranch — in both cases the caller sends no message.
func (h *Handler) Evaluate(ctx context.Context, owner, repo, baseBranch string, number int) (*Result, error) {
	if !h.cfg.Enabled {
		return nil, nil
	}
	rule, ok := h.cfg.Rules[baseBranch]
	if !ok || len(rule) == 0 {
		h.logger.Debug().Str("base", baseBranch).Msg("approval: branch not tracked, skipping")
		return nil, nil
	}

	reviews, err := h.gh.ListPullRequestReviews(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("approval: list reviews: %w", err)
	}

	approvers, changesRequested := latestDecisions(reviews)

	// Count approvals per role, resolving each approver's team memberships
	// and crediting them toward every role the rule asks for.
	counts := make(map[string]int, len(rule))
	for _, user := range approvers {
		for role := range rule {
			teamSlug, ok := h.cfg.Teams[role]
			if !ok {
				return nil, fmt.Errorf("approval: rule for %q references undefined role %q", baseBranch, role)
			}
			member, err := h.gh.IsTeamMember(ctx, h.cfg.Org, teamSlug, user)
			if err != nil {
				return nil, fmt.Errorf("approval: resolve membership: %w", err)
			}
			if member {
				counts[role]++
			}
		}
	}

	// Build the per-role status sorted by role name for stable output.
	roles := make([]RoleStatus, 0, len(rule))
	for role := range rule {
		roles = append(roles, RoleStatus{Role: role, Have: counts[role], Need: rule[role]})
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Role < roles[j].Role })

	satisfied := len(changesRequested) == 0
	for _, r := range roles {
		if !r.Met() {
			satisfied = false
		}
	}

	h.logger.Info().
		Str("repo", owner+"/"+repo).
		Int("pr", number).
		Str("base", baseBranch).
		Bool("satisfied", satisfied).
		Msg("approval progress evaluated")
	return &Result{Satisfied: satisfied, Roles: roles, ChangesRequested: changesRequested}, nil
}

// latestDecisions reduces a review list to the set of users whose most recent
// meaningful review is an approval, and those whose most recent is a changes
// request. COMMENTED reviews don't change a user's standing and DISMISSED ones
// clear it, so both are ignored/dropped. Reviews arrive chronologically, so the
// last state seen per user wins.
func latestDecisions(reviews []githubapp.Review) (approvers, changesRequested []string) {
	state := make(map[string]string)
	for _, r := range reviews {
		switch strings.ToUpper(r.State) {
		case "APPROVED", "CHANGES_REQUESTED":
			state[r.User] = strings.ToUpper(r.State)
		case "DISMISSED":
			// A dismissed review no longer counts for or against the user.
			delete(state, r.User)
		}
	}
	for user, s := range state {
		if s == "APPROVED" {
			approvers = append(approvers, user)
		} else {
			changesRequested = append(changesRequested, user)
		}
	}
	sort.Strings(approvers)
	sort.Strings(changesRequested)
	return approvers, changesRequested
}
