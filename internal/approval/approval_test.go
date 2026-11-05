package approval

import (
	"context"
	"testing"

	"hedwig/internal/githubapp"
	"hedwig/internal/githubapp/githubapptest"

	"github.com/rs/zerolog"
)

var testPR = PR{Owner: "acme", OwnerID: 1, Repo: "widgets", Base: "main", Number: 7}

func TestEvaluateDisabled(t *testing.T) {
	gh := githubapptest.New()
	gh.BranchRules = githubapp.BranchApprovalRules{RequiredApprovals: 1}
	res, err := New(gh, false, zerolog.Nop()).Evaluate(context.Background(), testPR)
	if err != nil || res != nil {
		t.Fatalf("Evaluate() = %v, %v; want nil, nil when disabled", res, err)
	}
}

func TestEvaluateNoRulesetSkips(t *testing.T) {
	gh := githubapptest.New()
	gh.Reviews = []githubapp.Review{{User: "alice", State: "APPROVED"}}
	res, err := New(gh, true, zerolog.Nop()).Evaluate(context.Background(), testPR)
	if err != nil || res != nil {
		t.Fatalf("Evaluate() = %v, %v; want nil, nil for a branch without approval rules", res, err)
	}
}

func TestEvaluateTotalAndTeams(t *testing.T) {
	gh := githubapptest.New()
	gh.BranchRules = githubapp.BranchApprovalRules{
		RequiredApprovals: 2,
		Teams: []githubapp.TeamApprovalRule{
			{TeamID: 10, MinApprovals: 1},
			{TeamID: 20, MinApprovals: 1},
			{TeamID: 20, MinApprovals: 2}, // stricter duplicate wins
		},
	}
	gh.TeamSlugs = map[int64]string{10: "infra-team", 20: "developer-team"}
	gh.Reviews = []githubapp.Review{
		{User: "alice", State: "APPROVED"},
		{User: "bob", State: "APPROVED"},
		{User: "carol", State: "COMMENTED"},
	}
	gh.TeamMembers = map[string]bool{
		"developer-team/alice": true,
		"developer-team/bob":   true,
		"infra-team/bob":       true,
	}

	res, err := New(gh, true, zerolog.Nop()).Evaluate(context.Background(), testPR)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	want := []RoleStatus{
		{Role: "approvals", Have: 2, Need: 2},
		{Role: "developer-team", Have: 2, Need: 2},
		{Role: "infra-team", Have: 1, Need: 1},
	}
	if len(res.Roles) != len(want) {
		t.Fatalf("Roles = %+v, want %+v", res.Roles, want)
	}
	for i := range want {
		if res.Roles[i] != want[i] {
			t.Errorf("Roles[%d] = %+v, want %+v", i, res.Roles[i], want[i])
		}
	}
	if !res.Satisfied {
		t.Error("Satisfied = false, want true")
	}
}

func TestEvaluateUnmetAndChangesRequested(t *testing.T) {
	gh := githubapptest.New()
	gh.BranchRules = githubapp.BranchApprovalRules{Teams: []githubapp.TeamApprovalRule{{TeamID: 10, MinApprovals: 1}}}
	gh.TeamSlugs = map[int64]string{10: "infra-team"}
	gh.Reviews = []githubapp.Review{
		{User: "alice", State: "APPROVED"},
		{User: "dave", State: "CHANGES_REQUESTED"},
	}

	res, err := New(gh, true, zerolog.Nop()).Evaluate(context.Background(), testPR)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if res.Satisfied {
		t.Error("Satisfied = true, want false")
	}
	if len(res.Roles) != 1 || res.Roles[0] != (RoleStatus{Role: "infra-team", Have: 0, Need: 1}) {
		t.Errorf("Roles = %+v, want only infra-team 0/1", res.Roles)
	}
	if len(res.ChangesRequested) != 1 || res.ChangesRequested[0] != "dave" {
		t.Errorf("ChangesRequested = %v, want [dave]", res.ChangesRequested)
	}
}

func TestLatestDecisionsDismissedClears(t *testing.T) {
	approvers, changes := latestDecisions([]githubapp.Review{
		{User: "alice", State: "APPROVED"},
		{User: "alice", State: "DISMISSED"},
		{User: "bob", State: "CHANGES_REQUESTED"},
		{User: "bob", State: "APPROVED"},
	})
	if len(approvers) != 1 || approvers[0] != "bob" || len(changes) != 0 {
		t.Errorf("latestDecisions() = %v, %v; want [bob], []", approvers, changes)
	}
}
