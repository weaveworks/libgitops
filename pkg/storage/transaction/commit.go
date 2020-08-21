package transaction

import (
	"fmt"

	"github.com/fluxcd/go-git-providers/validation"
)

// CommitResult describes a result of a transaction.
type CommitResult interface {
	// GetAuthorName describes the author's name (as per git config)
	// +required
	GetAuthorName() string
	// GetAuthorEmail describes the author's email (as per git config)
	// +required
	GetAuthorEmail() string
	// GetTitle describes the change concisely, so it can be used as a commit message or PR title.
	// +required
	GetTitle() string
	// GetDescription contains optional extra information about the change.
	// +optional
	GetDescription() string

	// GetMessage returns GetTitle() followed by a newline and GetDescription(), if set.
	GetMessage() string
	// Validate validates that all required fields are set, and given data is valid.
	Validate() error
}

// GenericCommitResult implements CommitResult.
var _ CommitResult = &GenericCommitResult{}

// GenericCommitResult implements CommitResult.
type GenericCommitResult struct {
	// AuthorName describes the author's name (as per git config)
	// +required
	AuthorName string
	// AuthorEmail describes the author's email (as per git config)
	// +required
	AuthorEmail string
	// Title describes the change concisely, so it can be used as a commit message or PR title.
	// +required
	Title string
	// Description contains optional extra information about the change.
	// +optional
	Description string
}

func (r *GenericCommitResult) GetAuthorName() string {
	return r.AuthorName
}
func (r *GenericCommitResult) GetAuthorEmail() string {
	return r.AuthorEmail
}
func (r *GenericCommitResult) GetTitle() string {
	return r.Title
}
func (r *GenericCommitResult) GetDescription() string {
	return r.Description
}
func (r *GenericCommitResult) GetMessage() string {
	if len(r.Description) == 0 {
		return r.Title
	}
	return fmt.Sprintf("%s\n%s", r.Title, r.Description)
}
func (r *GenericCommitResult) Validate() error {
	v := validation.New("GenericCommitResult")
	if len(r.AuthorName) == 0 {
		v.Required("AuthorName")
	}
	if len(r.AuthorEmail) == 0 {
		v.Required("AuthorEmail")
	}
	if len(r.Title) == 0 {
		v.Required("Title")
	}
	return v.Error()
}
