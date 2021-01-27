package core

import (
	"context"
	"errors"
)

var versionRefKey = versionRefKeyImpl{}

type versionRefKeyImpl struct{}

// WithVersionRef attaches the given VersionRef to a Context (it
// overwrites if one already exists in ctx). The key for the ref
// is private in this package, so one must use this function to
// register it.
func WithVersionRef(ctx context.Context, ref VersionRef) context.Context {
	return context.WithValue(ctx, versionRefKey, ref)
}

// GetVersionRef returns the VersionRef attached to this context.
// If there is no attached VersionRef, or it is nil, a BranchRef
// with branch "" will be returned as the "zero value" of VersionRef.
func GetVersionRef(ctx context.Context) VersionRef {
	r, ok := ctx.Value(versionRefKey).(VersionRef)
	// Return default ref if none specified
	if r == nil || !ok {
		return NewBranchRef("")
	}
	return r
}

var ErrInvalidVersionRefType = errors.New("invalid version ref type")

// NewBranchRef creates a new VersionRef for a given branch. It is
// valid for the branch to be ""; in this case it means the "zero
// value", or unspecified branch to be more precise, where the caller
// can choose how to handle.
func NewBranchRef(branch string) VersionRef { return branchRef{branch} }

// NewCommitRef creates a new VersionRef for the given commit. The
// commit must uniquely define a certain revision precisely. It must
// not be an empty string.
func NewCommitRef(commit string) (VersionRef, error) {
	if len(commit) == 0 {
		return nil, errors.New("commit must not be an empty string")
	}
	return commitRef{commit}, nil
}

// MustNewCommitRef runs NewCommitRef, but panics on errors
func MustNewCommitRef(commit string) VersionRef {
	ref, err := NewCommitRef(commit)
	if err != nil {
		panic(err)
	}
	return ref
}

type branchRef struct{ branch string }

func (r branchRef) String() string { return r.branch }

// A branch is considered writable, as commits can be added to it by libgitops
func (branchRef) IsWritable() bool { return true }

// A branch is considered the zero value if the branch is an empty string,
// which it is e.g. when there was no VersionRef associated with a Context.
func (r branchRef) IsZeroValue() bool { return r.branch == "" }

type commitRef struct{ commit string }

func (r commitRef) String() string { return r.commit }

// A commit is not considered writable, as it is only a read snapshot of
// a specific point in time.
func (commitRef) IsWritable() bool { return false }

// IsZeroValue should always return false for commits; as commit is mandatory
// to be a non-empty string.
func (r commitRef) IsZeroValue() bool { return r.commit == "" }
