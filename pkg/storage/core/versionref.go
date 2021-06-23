package core

import (
	"context"
)

type VersionRefResolver interface {
	//IsImmutable(ref string) (bool, error)
	// Turns a branch name into a commit hash. If ref already is an existing commit, this is a no-op.
	ResolveVersionRef(ref string) (c Commit, immutableRef bool, err error)
}

type Commit string

/*type VersionRef2 string

 */

var versionRefKey = versionRefKeyImpl{}

type versionRefKeyImpl struct{}

// WithVersionRef attaches the given VersionRef to a Context (it
// overwrites if one already exists in ctx). The key for the ref
// is private in this package, so one must use this function to
// register it.
func WithVersionRef(ctx context.Context, ref string) context.Context {
	return context.WithValue(ctx, versionRefKey, ref)
}

// GetVersionRef returns the VersionRef attached to this context.
// If there is no attached VersionRef, or it is nil, a BranchRef
// with branch "" will be returned as the "zero value" of VersionRef.
func GetVersionRef(ctx context.Context) string {
	r, ok := ctx.Value(versionRefKey).(string)
	// Return default ref if none specified
	if !ok {
		return ""
	}
	return r
}

/*
// NewMutableVersionRef creates a new VersionRef for a given branch. It is
// valid for the branch to be ""; in this case it means the "zero
// value", or unspecified branch to be more precise, where the caller
// can choose how to handle.
func NewMutableVersionRef(ref string) VersionRef {
	return versionRef{
		ref:       ref,
		immutable: false,
	}
}

func WithMutableVersionRef(ctx context.Context, ref string) context.Context {
	return WithVersionRef(ctx, NewMutableVersionRef(ref))
}

func NewImmutableVersionRef(ref string) VersionRef {
	return versionRef{
		ref:       ref,
		immutable: false,
	}
}

func WithImmutableVersionRef(ctx context.Context, ref string) context.Context {
	return WithVersionRef(ctx, NewImmutableVersionRef(ref))
}

type versionRef struct {
	ref       string
	immutable bool
}

func (r versionRef) VersionRef() string { return r.ref }

// A branch is considered the zero value if the branch is an empty string,
// which it is e.g. when there was no VersionRef associated with a Context.
func (r versionRef) IsZeroValue() bool { return r.ref == "" }

func (r versionRef) IsImmutable() bool { return r.immutable }

func NewLockedVersionRef(mutable, immutable VersionRef) LockedVersionRef {
	if !immutable.IsImmutable() {
		panic("NewLockedVersionRef: immutable VersionRef must be immutable")
	}
	return lockedVersionRef{
		mutable:   mutable,
		immutable: immutable,
	}
}

type lockedVersionRef struct {
	mutable, immutable VersionRef
}

func (r lockedVersionRef) VersionRef() string       { return r.mutable.VersionRef() }
func (r lockedVersionRef) IsZeroValue() bool        { return r.mutable.IsZeroValue() }
func (r lockedVersionRef) IsImmutable() bool        { return r.mutable.IsImmutable() }
func (r lockedVersionRef) ImmutableRef() VersionRef { return r.immutable }
*/
