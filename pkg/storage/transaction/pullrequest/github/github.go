package github

import (
	"context"
	"errors"
	"fmt"

	"github.com/fluxcd/go-git-providers/github"
	"github.com/fluxcd/go-git-providers/gitprovider"
	gogithub "github.com/google/go-github/v32/github"
	"github.com/weaveworks/libgitops/pkg/storage/transaction"
)

// TODO: This package should really only depend on go-git-providers' abstraction interface

var ErrProviderNotSupported = errors.New("only the Github go-git-providers provider is supported at the moment")

// NewGitHubPRProvider returns a new transaction.PullRequestProvider from a gitprovider.Client.
func NewGitHubPRProvider(c gitprovider.Client) (transaction.PullRequestProvider, error) {
	// Make sure a Github client was passed
	if c.ProviderID() != github.ProviderID {
		return nil, ErrProviderNotSupported
	}
	return &prCreator{c}, nil
}

type prCreator struct {
	c gitprovider.Client
}

func (c *prCreator) CreatePullRequest(ctx context.Context, spec transaction.PullRequestSpec) error {
	// First, validate the input
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("given PullRequestSpec wasn't valid")
	}

	// Use the "raw" go-github client to do this
	ghClient := c.c.Raw().(*gogithub.Client)

	// Helper variables
	owner := spec.GetRepositoryRef().GetIdentity()
	repo := spec.GetRepositoryRef().GetRepository()
	var body *string
	if spec.GetDescription() != "" {
		body = gogithub.String(spec.GetDescription())
	}

	// Create the Pull Request
	pr, _, err := ghClient.PullRequests.Create(ctx, owner, repo, &gogithub.NewPullRequest{
		Head:  gogithub.String(spec.GetMergeBranch()),
		Base:  gogithub.String(spec.GetMainBranch()),
		Title: gogithub.String(spec.GetTitle()),
		Body:  body,
	})
	if err != nil {
		return err
	}

	// If spec.GetMilestone() is set, fetch the ID of the milestone
	// Only set milestoneID to non-nil if specified
	var milestoneID *int
	if len(spec.GetMilestone()) != 0 {
		milestoneID, err = getMilestoneID(ctx, ghClient, owner, repo, spec.GetMilestone())
		if err != nil {
			return err
		}
	}

	// Only set assignees to non-nil if specified
	var assignees *[]string
	if a := spec.GetAssignees(); len(a) != 0 {
		assignees = &a
	}

	// Only set labels to non-nil if specified
	var labels *[]string
	if l := spec.GetLabels(); len(l) != 0 {
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
