package transaction

import (
	"context"

	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/fluxcd/go-git-providers/validation"
)

// PullRequestResult can be returned from a TransactionFunc instead of a CommitResult, if
// a PullRequest is desired to be created by the PullRequestProvider.
type PullRequestResult interface {
	// PullRequestResult is a superset of CommitResult
	CommitResult

	// GetLabels specifies what labels should be applied on the PR.
	// +optional
	GetLabels() []string
	// GetAssignees specifies what user login names should be assigned to this PR.
	// Note: Only users with "pull" access or more can be assigned.
	// +optional
	GetAssignees() []string
	// GetMilestone specifies what milestone this should be attached to.
	// +optional
	GetMilestone() string
}

// GenericPullRequestResult implements PullRequestResult.
var _ PullRequestResult = &GenericPullRequestResult{}

// GenericPullRequestResult implements PullRequestResult.
type GenericPullRequestResult struct {
	// GenericPullRequestResult is a superset of a CommitResult.
	CommitResult

	// Labels specifies what labels should be applied on the PR.
	// +optional
	Labels []string
	// Assignees specifies what user login names should be assigned to this PR.
	// Note: Only users with "pull" access or more can be assigned.
	// +optional
	Assignees []string
	// Milestone specifies what milestone this should be attached to.
	// +optional
	Milestone string
}

func (r *GenericPullRequestResult) GetLabels() []string {
	return r.Labels
}
func (r *GenericPullRequestResult) GetAssignees() []string {
	return r.Assignees
}
func (r *GenericPullRequestResult) GetMilestone() string {
	return r.Milestone
}
func (r *GenericPullRequestResult) Validate() error {
	v := validation.New("GenericPullRequestResult")
	// Just validate the "inner" object
	v.Append(r.CommitResult.Validate(), r.CommitResult, "CommitResult")
	return v.Error()
}

// PullRequestSpec is the messaging interface between the TransactionStorage, and the
// PullRequestProvider. The PullRequestSpec contains all the needed information for creating
// a Pull Request successfully.
type PullRequestSpec interface {
	// PullRequestSpec is a superset of PullRequestResult.
	PullRequestResult

	// GetMainBranch returns the main branch of the repository.
	// +required
	GetMainBranch() string
	// GetMergeBranch returns the branch that is pending to be merged into main with this PR.
	// +required
	GetMergeBranch() string
	// GetMergeBranch returns the branch that is pending to be merged into main with this PR.
	// +required
	GetRepositoryRef() gitprovider.RepositoryRef
}

// GenericPullRequestSpec implements PullRequestSpec.
type GenericPullRequestSpec struct {
	// GenericPullRequestSpec is a superset of PullRequestResult.
	PullRequestResult

	// MainBranch returns the main branch of the repository.
	// +required
	MainBranch string
	// MergeBranch returns the branch that is pending to be merged into main with this PR.
	// +required
	MergeBranch string
	// RepositoryRef returns the branch that is pending to be merged into main with this PR.
	// +required
	RepositoryRef gitprovider.RepositoryRef
}

func (r *GenericPullRequestSpec) GetMainBranch() string {
	return r.MainBranch
}
func (r *GenericPullRequestSpec) GetMergeBranch() string {
	return r.MergeBranch
}
func (r *GenericPullRequestSpec) GetRepositoryRef() gitprovider.RepositoryRef {
	return r.RepositoryRef
}
func (r *GenericPullRequestSpec) Validate() error {
	v := validation.New("GenericPullRequestSpec")
	// Just validate the "inner" object
	v.Append(r.PullRequestResult.Validate(), r.PullRequestResult, "PullRequestResult")

	if len(r.MainBranch) == 0 {
		v.Required("MainBranch")
	}
	if len(r.MergeBranch) == 0 {
		v.Required("MergeBranch")
	}
	if r.RepositoryRef == nil {
		v.Required("RepositoryRef")
	}
	return v.Error()
}

// PullRequestProvider is an interface for providers that can create so-called "Pull Requests",
// as popularized by Git. A Pull Request is a formal ask for a branch to be merged into the main one.
// It can be UI-based, as in GitHub and GitLab, or it can be using some other method.
type PullRequestProvider interface {
	// CreatePullRequest creates a Pull Request using the given specification.
	CreatePullRequest(ctx context.Context, spec PullRequestSpec) error
}
