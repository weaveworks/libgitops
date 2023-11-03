package pr

import "github.com/weaveworks/libgitops/pkg/storage/commit"

// Request can be returned when committing a transaction instead of a
// commit.Request, if the intention is to create a PR in e.g. GitHub.
type Request interface {
	// PullRequest is a superset of commit.Request
	commit.Request
	PullRequest() Metadata
}

type Metadata interface {
	// TargetBranch specifies what branch the Pull Request head branch should
	// be merged into.
	// +required
	TargetBranch() string
	// Labels specifies what labels should be applied on the PR.
	// +optional
	Labels() []string
	// Assignees specifies what user login names should be assigned to this PR.
	// Note: Only users with "pull" access or more can be assigned.
	// +optional
	Assignees() []string
	// Milestone specifies what milestone this should be attached to.
	// +optional
	Milestone() string
}
