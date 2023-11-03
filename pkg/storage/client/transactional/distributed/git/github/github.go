package github

import (
	"context"
	"errors"
	"fmt"

	"github.com/fluxcd/go-git-providers/github"
	"github.com/fluxcd/go-git-providers/gitprovider"
	gogithub "github.com/google/go-github/v32/github"
	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/commit"
	"github.com/weaveworks/libgitops/pkg/storage/commit/pr"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// PullRequest implements pr.Request.
var _ pr.Request = PullRequest{}

// PullRequest implements PullRequest.
type PullRequest struct {
	// PullRequest is a superset of any Commit.
	commit.Request

	// TargetBranch specifies what branch the Pull Request head branch should
	// be merged into.
	// +required
	TargetBranch string
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

func (r PullRequest) PullRequest() pr.Metadata {
	return &metadata{&r.Labels, &r.Assignees, &r.TargetBranch, &r.Milestone}
}

func (r PullRequest) Validate() error {
	root := field.NewPath("github.PullRequest")
	allErrs := field.ErrorList{}
	if err := r.Request.Validate(); err != nil {
		allErrs = append(allErrs, field.Invalid(root.Child("Request"), r.Request, err.Error()))
	}
	return allErrs.ToAggregate()
}

type metadata struct {
	labels, assignees       *[]string
	targetBranch, milestone *string
}

func (m *metadata) TargetBranch() string { return *m.targetBranch }
func (m *metadata) Labels() []string     { return *m.labels }
func (m *metadata) Assignees() []string  { return *m.assignees }
func (m *metadata) Milestone() string    { return *m.milestone }

// TODO: This package should really only depend on go-git-providers' abstraction interface

var ErrProviderNotSupported = errors.New("only the Github go-git-providers provider is supported at the moment")

// NewGitHubPRCommitHandler returns a new transactional.CommitHandler from a gitprovider.Client.
func NewGitHubPRCommitHandler(c gitprovider.Client, repoRef gitprovider.RepositoryRef) (transactional.CommitHook, error) {
	// Make sure a Github client was passed
	if c.ProviderID() != github.ProviderID {
		return nil, ErrProviderNotSupported
	}
	return &prCreator{c, repoRef}, nil
}

type prCreator struct {
	c       gitprovider.Client
	repoRef gitprovider.RepositoryRef
}

func (c *prCreator) PreCommitHook(ctx context.Context, info transactional.TxInfo, req commit.Request) error {
	return nil
}

func (c *prCreator) PostCommitHook(ctx context.Context, info transactional.TxInfo, req commit.Request) error {
	// First, validate the input
	if err := req.Validate(); err != nil {
		return fmt.Errorf("given commit.Request wasn't valid: %v", err)
	}

	prCommit, ok := req.(pr.Request)
	if !ok {
		return nil
	}

	// Use the "raw" go-github client to do this
	ghClient := c.c.Raw().(*gogithub.Client)

	// Helper variables
	owner := c.repoRef.GetIdentity()
	repo := c.repoRef.GetRepository()
	var body *string
	if prCommit.Message().Description() != "" {
		body = gogithub.String(prCommit.Message().Description())
	}

	// Create the Pull Request
	prPayload := &gogithub.NewPullRequest{
		Head:  gogithub.String(info.Target.DestBranch()),
		Base:  gogithub.String(prCommit.PullRequest().TargetBranch()),
		Title: gogithub.String(prCommit.Message().Title()),
		Body:  body,
	}
	logrus.Infof("GitHub PR payload: %+v", prPayload)
	pr, _, err := ghClient.PullRequests.Create(ctx, owner, repo, prPayload)
	if err != nil {
		return err
	}

	// If spec.GetMilestone() is set, fetch the ID of the milestone
	// Only set milestoneID to non-nil if specified
	var milestoneID *int
	if len(prCommit.PullRequest().Milestone()) != 0 {
		milestoneID, err = getMilestoneID(ctx, ghClient, owner, repo, prCommit.PullRequest().Milestone())
		if err != nil {
			return err
		}
	}

	// Only set assignees to non-nil if specified
	var assignees *[]string
	if a := prCommit.PullRequest().Assignees(); len(a) != 0 {
		assignees = &a
	}

	// Only set labels to non-nil if specified
	var labels *[]string
	if l := prCommit.PullRequest().Labels(); len(l) != 0 {
		labels = &l
	}

	// Only PATCH the PR if any of the fields were set
	if milestoneID != nil || assignees != nil || labels != nil {
		_, _, err := ghClient.Issues.Edit(ctx, owner, repo, pr.GetNumber(), &gogithub.IssueRequest{
			Milestone: milestoneID,
			Assignees: assignees,
			Labels:    labels,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func getMilestoneID(ctx context.Context, c *gogithub.Client, owner, repo, milestoneName string) (*int, error) {
	// List all milestones in the repo
	// TODO: This could/should use pagination
	milestones, _, err := c.Issues.ListMilestones(ctx, owner, repo, &gogithub.MilestoneListOptions{
		State: "all",
	})
	if err != nil {
		return nil, err
	}
	// Loop through all milestones, search for one with the right name
	for _, milestone := range milestones {
		// Only consider a milestone with the right name
		if milestone.GetTitle() != milestoneName {
			continue
		}
		// Validate nil to avoid panics
		if milestone.Number == nil {
			return nil, fmt.Errorf("didn't expect milestone Number to be nil: %v", milestone)
		}
		// Return the Milestone number
		return milestone.Number, nil
	}
	return nil, fmt.Errorf("couldn't find milestone with name: %s", milestoneName)
}
