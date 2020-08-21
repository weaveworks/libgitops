package gitdir

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/fluxcd/go-git-providers/gitprovider"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	// ErrNotStarted happens if you try to operate on the gitDirectory before you have started
	// it with StartCheckoutLoop.
	ErrNotStarted = errors.New("the gitDirectory hasn't been started (and hence, cloned) yet")
	// ErrCannotWriteToReadOnly happens if you try to do a write operation for a non-authenticated Git repo.
	ErrCannotWriteToReadOnly = errors.New("the gitDirectory is read-only, cannot write")
)

const (
	defaultBranch   = "master"
	defaultRemote   = "origin"
	defaultInterval = 30 * time.Second
	defaultTimeout  = 1 * time.Minute
)

// GitDirectoryOptions provides options for the gitDirectory.
// TODO: Refactor this into the controller-runtime Options factory pattern.
type GitDirectoryOptions struct {
	// Options
	Branch   string        // default "master"
	Interval time.Duration // default 30s
	Timeout  time.Duration // default 1m
	// TODO: Support folder prefixes

	// Authentication
	AuthMethod AuthMethod
}

func (o *GitDirectoryOptions) Default() {
	if o.Branch == "" {
		o.Branch = defaultBranch
	}
	if o.Interval == 0 {
		o.Interval = defaultInterval
	}
	if o.Timeout == 0 {
		o.Timeout = defaultTimeout
	}
}

// GitDirectory is an abstraction layer for a temporary Git clone. It pulls
// and checks out new changes periodically in the background. It also allows
// high-level access to write operations, like creating a new branch, committing,
// and pushing.
type GitDirectory interface {
	// Dir returns the backing temporary directory of the git clone.
	Dir() string
	// MainBranch returns the configured main branch.
	MainBranch() string
	// RepositoryRef returns the repository reference.
	RepositoryRef() gitprovider.RepositoryRef

	// StartCheckoutLoop clones the repo synchronously, and then starts the checkout loop non-blocking.
	// If the checkout loop has been started already, this is a no-op.
	StartCheckoutLoop() error
	// Suspend waits for any pending transactions or operations, and then locks the internal mutex so that
	// no other operations can start. This means the periodic background checkout loop will momentarily stop.
	Suspend()
	// Resume unlocks the mutex locked in Suspend(), so that other Git operations, like the background checkout
	// loop can resume its operation.
	Resume()

	// Pull performs a pull & checkout to the latest revision.
	// ErrNotStarted is returned if the repo hasn't been cloned yet.
	Pull(ctx context.Context) error

	// CheckoutNewBranch creates a new branch and checks out to it.
	// ErrNotStarted is returned if the repo hasn't been cloned yet.
	CheckoutNewBranch(branchName string) error
	// CheckoutMainBranch goes back to the main branch.
	// ErrNotStarted is returned if the repo hasn't been cloned yet.
	CheckoutMainBranch() error

	// Commit creates a commit of all changes in the current worktree with the given parameters.
	// It also automatically pushes the branch after the commit.
	// ErrNotStarted is returned if the repo hasn't been cloned yet.
	// ErrCannotWriteToReadOnly is returned if opts.AuthMethod wasn't provided.
	Commit(ctx context.Context, authorName, authorEmail, msg string) error
	// CommitChannel is a channel to where new observed Git SHAs are written.
	CommitChannel() chan string

	// Cleanup terminates any pending operations, and removes the temporary directory.
	Cleanup() error
}

// Create a new GitDirectory implementation. In order to start using this, run StartCheckoutLoop().
func NewGitDirectory(repoRef gitprovider.RepositoryRef, opts GitDirectoryOptions) (GitDirectory, error) {
	log.Info("Initializing the Git repo...")

	// Default the options
	opts.Default()

	// Create a temporary directory for the clone
	tmpDir, err := ioutil.TempDir("", "libgitops")
	if err != nil {
		return nil, err
	}
	log.Debugf("Created temporary directory for the git clone at %q", tmpDir)

	d := &gitDirectory{
		repoRef:             repoRef,
		GitDirectoryOptions: opts,
		cloneDir:            tmpDir,
		// TODO: This needs to be large, otherwise it can start blocking unnecessarily if nobody reads it
		commitChan: make(chan string, 1024),
		lock:       &sync.Mutex{},
	}
	// Set up the parent context for this class. d.cancel() is called only at Cleanup()
	d.ctx, d.cancel = context.WithCancel(context.Background())

	log.Trace("URL endpoint parsed and authentication method chosen")

	if d.canWrite() {
		log.Infof("Running in read-write mode, will commit back current status to the repo")
	} else {
		log.Infof("Running in read-only mode, won't write status back to the repo")
	}

	return d, nil
}

// gitDirectory is an implementation which keeps a directory
type gitDirectory struct {
	// user-specified options
	repoRef gitprovider.RepositoryRef
	GitDirectoryOptions

	// the temporary directory used for the clone
	cloneDir string

	// go-git objects. wt is the worktree of the repo, persistent during the lifetime of repo.
	repo *git.Repository
	wt   *git.Worktree

	// latest known commit to the system
	lastCommit string
	// events channel from new commits
	commitChan chan string

	// the context and its cancel function for the lifetime of this struct (until Cleanup())
	ctx    context.Context
	cancel context.CancelFunc
	// the lock for git operations (so pushing and pulling aren't done simultaneously)
	lock *sync.Mutex
}

func (d *gitDirectory) Dir() string {
	return d.cloneDir
}

func (d *gitDirectory) MainBranch() string {
	return d.Branch
}

func (d *gitDirectory) RepositoryRef() gitprovider.RepositoryRef {
	return d.repoRef
}

// StartCheckoutLoop clones the repo synchronously, and then starts the checkout loop non-blocking.
// If the checkout loop has been started already, this is a no-op.
func (d *gitDirectory) StartCheckoutLoop() error {
	if d.wt != nil {
		return nil // already initialized
	}
	// First, clone the repo
	if err := d.clone(); err != nil {
		return err
	}
	go d.checkoutLoop()
	return nil
}

func (d *gitDirectory) Suspend() {
	d.lock.Lock()
}

func (d *gitDirectory) Resume() {
	d.lock.Unlock()
}

func (d *gitDirectory) CommitChannel() chan string {
	return d.commitChan
}

func (d *gitDirectory) checkoutLoop() {
	log.Info("Starting the checkout loop...")

	wait.NonSlidingUntilWithContext(d.ctx, func(_ context.Context) {

		log.Trace("checkoutLoop: Will perform pull operation")
		// Perform a pull & checkout of the new revision
		if err := d.Pull(d.ctx); err != nil {
			log.Errorf("checkoutLoop: git pull failed with error: %v", err)
			return
		}

	}, d.Interval)
	log.Info("Exiting the checkout loop...")
}

func (d *gitDirectory) cloneURL() string {
	return d.repoRef.GetCloneURL(d.AuthMethod.TransportType())
}

func (d *gitDirectory) canWrite() bool {
	return d.AuthMethod != nil
}

// verifyRead makes sure it's ok to start a read-something-from-git process
func (d *gitDirectory) verifyRead() error {
	// Safeguard against not starting yet
	if d.wt == nil {
		return fmt.Errorf("cannot pull: %w", ErrNotStarted)
	}
	return nil
}

// verifyWrite makes sure it's ok to start a write-something-to-git process
func (d *gitDirectory) verifyWrite() error {
	// We need all read privileges first
	if err := d.verifyRead(); err != nil {
		return err
	}
	// Make sure we don't write to a possibly read-only repo
	if !d.canWrite() {
		return ErrCannotWriteToReadOnly
	}
	return nil
}

func (d *gitDirectory) clone() error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	log.Infof("Starting to clone the repository %s with timeout %s", d.repoRef, d.Timeout)
	// Do a clone operation to the temporary directory, with a timeout
	err := d.contextWithTimeout(d.ctx, func(ctx context.Context) error {
		var err error
		d.repo, err = git.PlainCloneContext(ctx, d.Dir(), false, &git.CloneOptions{
			URL:           d.cloneURL(),
			Auth:          d.AuthMethod,
			RemoteName:    defaultRemote,
			ReferenceName: plumbing.NewBranchReferenceName(d.Branch),
			SingleBranch:  true,
			NoCheckout:    false,
			//Depth:             1, // ref: https://github.com/src-d/go-git/issues/1143
			RecurseSubmodules: 0,
			Progress:          nil,
			Tags:              git.NoTags,
		})
		return err
	})
	// Handle errors
	switch err {
	case nil:
		// no-op, just continue.
	case context.DeadlineExceeded:
		return fmt.Errorf("git clone operation took longer than deadline %s", d.Timeout)
	case context.Canceled:
		log.Tracef("context was cancelled")
		return nil // if Cleanup() was called, just exit the goroutine
	default:
		return fmt.Errorf("git clone error: %v", err)
	}

	// Populate the worktree pointer
	d.wt, err = d.repo.Worktree()
	if err != nil {
		return fmt.Errorf("git get worktree error: %v", err)
	}

	// Get the latest HEAD commit and report it to the user
	ref, err := d.repo.Head()
	if err != nil {
		return err
	}

	d.observeCommit(ref.Hash())
	return nil
}

func (d *gitDirectory) Pull(ctx context.Context) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// Make sure it's okay to read
	if err := d.verifyRead(); err != nil {
		return err
	}

	// Perform the git pull operation using the timeout
	err := d.contextWithTimeout(ctx, func(innerCtx context.Context) error {
		log.Trace("checkoutLoop: Starting pull operation")
		return d.wt.PullContext(innerCtx, &git.PullOptions{
			Auth:         d.AuthMethod,
			SingleBranch: true,
		})
	})
	// Handle errors
	switch err {
	case nil, git.NoErrAlreadyUpToDate:
		// no-op, just continue. Allow the git.NoErrAlreadyUpToDate error
	case context.DeadlineExceeded:
		return fmt.Errorf("git pull operation took longer than deadline %s", d.Timeout)
	case context.Canceled:
		log.Tracef("context was cancelled")
		return nil // if Cleanup() was called, just exit the goroutine
	default:
		return fmt.Errorf("failed to pull: %v", err)
	}

	log.Trace("checkoutLoop: Pulled successfully")

	// get current head
	ref, err := d.repo.Head()
	if err != nil {
		return err
	}

	// check if we changed commits
	if d.lastCommit != ref.Hash().String() {
		// Notify upstream that we now have a new commit, and allow writing again
		d.observeCommit(ref.Hash())
	}

	return nil
}

func (d *gitDirectory) CheckoutNewBranch(branchName string) error {
	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	return d.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})
}

func (d *gitDirectory) CheckoutMainBranch() error {
	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	// Best-effort clean
	_ = d.wt.Clean(&git.CleanOptions{
		Dir: true,
	})
	// Force-checkout the main branch
	return d.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(d.Branch),
		Force:  true,
	})
}

// observeCommit sets the lastCommit variable so that we know the latest state
func (d *gitDirectory) observeCommit(commit plumbing.Hash) {
	d.lastCommit = commit.String()
	d.commitChan <- commit.String()
	log.Infof("New commit observed on branch %q: %s", d.Branch, commit)
}

// Commit creates a commit of all changes in the current worktree with the given parameters.
// It also automatically pushes the branch after the commit.
// ErrNotStarted is returned if the repo hasn't been cloned yet.
// ErrCannotWriteToReadOnly is returned if opts.AuthMethod wasn't provided.
func (d *gitDirectory) Commit(ctx context.Context, authorName, authorEmail, msg string) error {
	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	s, err := d.wt.Status()
	if err != nil {
		return fmt.Errorf("git status failed: %v", err)
	}
	if s.IsClean() {
		log.Debugf("No changed files in git repo, nothing to commit...")
		return nil
	}

	// Do a commit and push
	log.Debug("commitLoop: Committing all local changes")
	hash, err := d.wt.Commit(msg, &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit error: %v", err)
	}

	// Perform the git push operation using the timeout
	err = d.contextWithTimeout(ctx, func(innerCtx context.Context) error {
		log.Debug("commitLoop: Will push with timeout")
		return d.repo.PushContext(innerCtx, &git.PushOptions{
			Auth: d.AuthMethod,
		})
	})
	// Handle errors
	switch err {
	case nil, git.NoErrAlreadyUpToDate:
		// no-op, just continue. Allow the git.NoErrAlreadyUpToDate error
	case context.DeadlineExceeded:
		return fmt.Errorf("git push operation took longer than deadline %s", d.Timeout)
	case context.Canceled:
		log.Tracef("context was cancelled")
		return nil // if Cleanup() was called, just exit the goroutine
	default:
		return fmt.Errorf("failed to push: %v", err)
	}

	// Notify upstream that we now have a new commit, and allow writing again
	log.Infof("A new commit with the actual state has been created and pushed to the origin: %q", hash)
	d.observeCommit(hash)
	return nil
}

func (d *gitDirectory) contextWithTimeout(ctx context.Context, fn func(context.Context) error) error {
	// Create a new context with a timeout. The push operation either succeeds in time, times out,
	// or is cancelled by Cleanup(). In case of a successful run, the context is always cancelled afterwards.
	ctx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	// Run the function using the context and cancel directly afterwards
	fnErr := fn(ctx)

	// Return the context error, if any, first so deadline/cancel signals can propagate.
	// Otherwise passthrough the error returned from the function.
	if ctx.Err() != nil {
		log.Debugf("operation context yielded error %v to be returned. Function error was: %v", ctx.Err(), fnErr)
		return ctx.Err()
	}
	return fnErr
}

// Cleanup cancels running goroutines and operations, and removes the temporary clone directory
func (d *gitDirectory) Cleanup() error {
	// Cancel the context for the two running goroutines, and any possible long-running operations
	d.cancel()

	// Remove the temporary directory
	if err := os.RemoveAll(d.Dir()); err != nil {
		log.Errorf("Failed to clean up temp git directory: %v", err)
		return err
	}
	return nil
}
