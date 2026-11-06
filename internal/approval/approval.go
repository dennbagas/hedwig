// Package approval computes a branch-aware view of a pull request's approval
// progress, for notification purposes only.
//
// The requirements are read live from GitHub repository rulesets (including
// org-level rulesets that target the repo), so nothing is configured in
// Hedwig: a new repo picks up whatever rulesets apply to it, and a repo/branch
// with no pull_request rule is simply not tracked. Enforcement stays with
// GitHub; this package only reads the current reviews, counts them against
// the active rules for the PR's base branch, and returns a Result so the
// caller can post a progress message to Telegram/Slack.
//
// Two kinds of requirement are tracked:
//
//   - required_approving_review_count: a plain total of approving reviews,
//     reported as the "approvals" role — omitted when a team requirement
//     already implies it (total <= the largest team minimum).
//   - required_reviewers: a minimum number of approvals from members of a
//     specific team, reported under the team's slug.
//
// Conventions, applied consistently:
//
//   - Multi-team approvers: an approver counts toward every team they are an
//     active member of.
//   - Stale approvals: this is stateless and always evaluates the reviews
//     GitHub reports at call time, taking the latest review per user. When
//     "dismiss stale approvals" is on, dismissed reviews drop out of the count
//     automatically. A standing CHANGES_REQUESTED marks the PR unsatisfied.
//   - required_reviewers file_patterns are not evaluated: every team rule on
//     the branch is treated as applying to the PR.
package approval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"hedwig/internal/githubapp"

	"github.com/rs/zerolog"
)

// totalRole labels the plain required_approving_review_count requirement.
const totalRole = "approvals"

// Handler evaluates approval progress for a pull request.
type Handler struct {
	gh      githubapp.Client
	enabled bool
	logger  zerolog.Logger

	mu        sync.Mutex
	teamSlugs map[int64]string // team ID → slug; IDs are stable, so cached forever
}

// New returns a Handler. When enabled is false, Evaluate is a no-op.
func New(gh githubapp.Client, enabled bool, logger zerolog.Logger) *Handler {
	return &Handler{gh: gh, enabled: enabled, logger: logger, teamSlugs: map[int64]string{}}
}

// RoleStatus is one requirement's progress toward its required approval count.
type RoleStatus struct {
	Role string
	Have int
	Need int
}

// Met reports whether this requirement is satisfied.
func (r RoleStatus) Met() bool { return r.Have >= r.Need }

// Result is the outcome of an approval evaluation, returned so the caller can
// render a status notification. Roles lists the total requirement first (if
// any), then team requirements sorted by slug.
type Result struct {
	Satisfied        bool
	Roles            []RoleStatus
	ChangesRequested []string // reviewer logins with a standing changes-request
}

// Enabled reports whether tracking is active (so callers can skip work early).
func (h *Handler) Enabled() bool { return h.enabled }

// PR identifies the pull request to evaluate.
type PR struct {
	Owner   string // repo owner login (the org, for team lookups)
	OwnerID int64  // repo owner ID, needed to resolve team IDs from rulesets
	Repo    string
	Base    string // base branch name
	Number  int
}

// Evaluate reads the rulesets active on the PR's base branch, counts the
// current approvals, and returns per-requirement progress.
//
// Returns (nil, nil) when tracking is disabled or no ruleset requires
// approvals on the base branch — in both cases the caller sends no message.
func (h *Handler) Evaluate(ctx context.Context, pr PR) (*Result, error) {
	if !h.enabled {
		return nil, nil
	}

	rules, err := h.gh.GetBranchApprovalRules(ctx, pr.Owner, pr.Repo, pr.Base)
	if err != nil {
		return nil, fmt.Errorf("approval: get branch rules: %w", err)
	}
	if rules.RequiredApprovals == 0 && len(rules.Teams) == 0 {
		h.logger.Debug().Str("repo", pr.Owner+"/"+pr.Repo).Str("base", pr.Base).
			Msg("approval: no ruleset requires approvals, skipping")
		return nil, nil
	}

	reviews, err := h.gh.ListPullRequestReviews(ctx, pr.Owner, pr.Repo, pr.Number)
	if err != nil {
		return nil, fmt.Errorf("approval: list reviews: %w", err)
	}
	approvers, changesRequested := latestDecisions(reviews)

	// Several rulesets (or file-pattern entries) can name the same team; the
	// strictest minimum wins.
	teamNeed := map[int64]int{}
	maxTeamNeed := 0
	for _, t := range rules.Teams {
		if t.MinApprovals > teamNeed[t.TeamID] {
			teamNeed[t.TeamID] = t.MinApprovals
		}
		if t.MinApprovals > maxTeamNeed {
			maxTeamNeed = t.MinApprovals
		}
	}

	// The total is only worth showing when a team rule doesn't already imply
	// it: N approvals from one team are N distinct approvers, so a total of
	// <= the largest team minimum is satisfied whenever that team's is.
	var roles []RoleStatus
	if rules.RequiredApprovals > maxTeamNeed {
		roles = append(roles, RoleStatus{Role: totalRole, Have: len(approvers), Need: rules.RequiredApprovals})
	}
	var teamRoles []RoleStatus
	for teamID, need := range teamNeed {
		slug, err := h.teamSlug(ctx, pr.OwnerID, teamID)
		if err != nil {
			return nil, fmt.Errorf("approval: resolve team %d: %w", teamID, err)
		}
		have := 0
		for _, user := range approvers {
			member, err := h.gh.IsTeamMember(ctx, pr.Owner, slug, user)
			if err != nil {
				return nil, fmt.Errorf("approval: resolve membership: %w", err)
			}
			if member {
				have++
			}
		}
		teamRoles = append(teamRoles, RoleStatus{Role: slug, Have: have, Need: need})
	}
	sort.Slice(teamRoles, func(i, j int) bool { return teamRoles[i].Role < teamRoles[j].Role })
	roles = append(roles, teamRoles...)

	satisfied := len(changesRequested) == 0
	for _, r := range roles {
		if !r.Met() {
			satisfied = false
		}
	}

	h.logger.Info().
		Str("repo", pr.Owner+"/"+pr.Repo).
		Int("pr", pr.Number).
		Str("base", pr.Base).
		Bool("satisfied", satisfied).
		Msg("approval progress evaluated")
	return &Result{Satisfied: satisfied, Roles: roles, ChangesRequested: changesRequested}, nil
}

// teamSlug resolves a ruleset's team ID to its slug, caching the answer.
func (h *Handler) teamSlug(ctx context.Context, orgID, teamID int64) (string, error) {
	h.mu.Lock()
	slug, ok := h.teamSlugs[teamID]
	h.mu.Unlock()
	if ok {
		return slug, nil
	}
	slug, err := h.gh.GetTeamSlug(ctx, orgID, teamID)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	h.teamSlugs[teamID] = slug
	h.mu.Unlock()
	return slug, nil
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
