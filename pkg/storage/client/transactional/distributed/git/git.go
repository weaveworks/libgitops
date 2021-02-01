package git

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
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional/distributed"
)

var (
	// ErrNotStarted happens if you try to operate on the LocalClone before you have started
	// it with StartCheckoutLoop.
	ErrNotStarted = errors.New("the LocalClone hasn't been started (and hence, cloned) yet")
	// ErrCannotWriteToReadOnly happens if you try to do a write operation for a non-authenticated Git repo.
	ErrCannotWriteToReadOnly = errors.New("the LocalClone is read-only, cannot write")
)

const (
	defaultBranch = "master"
)

// LocalCloneOptions provides options for the LocalClone.
// TODO: Refactor this into the controller-runtime Options factory pattern.
type LocalCloneOptions struct {
	Branch string // default "master"

	// Authentication method. If unspecified, this clone is read-only.
	AuthMethod AuthMethod
}

func (o *LocalCloneOptions) Default() {
	if o.Branch == "" {
		o.Branch = defaultBranch
	}
}

// LocalClone is an implementation of both a Remote, and a BranchManager, for Git.
var _ transactional.BranchManager = &LocalClone{}
var _ distributed.Remote = &LocalClone{}

// Create a new Remote and BranchManager implementation using Git. The repo is cloned immediately
// in the constructor, you can use ctx to enforce a timeout for the clone.
func NewLocalClone(ctx context.Context, repoRef gitprovider.RepositoryRef, opts LocalCloneOptions) (*LocalClone, error) {
	log.Info("Initializing the Git repo...")

	// Default the options
	opts.Default()

	// Create a temporary directory for the clone
	tmpDir, err := ioutil.TempDir("", "libgitops")
	if err != nil {
		return nil, err
	}
	log.Debugf("Created temporary directory for the git clone at %q", tmpDir)

	d := &LocalClone{
		repoRef:     repoRef,
		opts:        opts,
		cloneDir:    tmpDir,
		lock:        &sync.Mutex{},
		commitHooks: &transactional.MultiCommitHook{},
		txHooks:     &transactional.MultiTransactionHook{},
	}

	log.Trace("URL endpoint parsed and authentication method chosen")

	if d.canWrite() {
		log.Infof("Running in read-write mode, will commit back current status to the repo")
	} else {
		log.Infof("Running in read-only mode, won't write status back to the repo")
	}

	// Clone the repo
	if err := d.clone(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// LocalClone is an implementation of both a Remote, and a BranchManager, for Git.
type LocalClone struct {
	// user-specified options
	repoRef gitprovider.RepositoryRef
	opts    LocalCloneOptions

	// the temporary directory used for the clone
	cloneDir string

	// go-git objects. wt is the worktree of the repo, persistent during the lifetime of repo.
	repo *git.Repository
	wt   *git.Worktree

	// the lock for git operations (so no ops are done simultaneously)
	lock *sync.Mutex

	commitHooks transactional.CommitHookChain
	txHooks     transactional.TransactionHookChain
}

func (d *LocalClone) CommitHookChain() transactional.CommitHookChain {
	return d.commitHooks
}

func (d *LocalClone) TransactionHookChain() transactional.TransactionHookChain {
	return d.txHooks
}

func (d *LocalClone) Dir() string {
	return d.cloneDir
}

func (d *LocalClone) MainBranch() string {
	return d.opts.Branch
}

func (d *LocalClone) RepositoryRef() gitprovider.RepositoryRef {
	return d.repoRef
}

func (d *LocalClone) canWrite() bool {
	return d.opts.AuthMethod != nil
}

// verifyRead makes sure it's ok to start a read-something-from-git process
func (d *LocalClone) verifyRead() error {
	// Safeguard against not starting yet
	if d.wt == nil {
		return fmt.Errorf("cannot pull: %w", ErrNotStarted)
	}
	return nil
}

// verifyWrite makes sure it's ok to start a write-something-to-git process
func (d *LocalClone) verifyWrite() error {
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

func (d *LocalClone) clone(ctx context.Context) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	cloneURL := d.repoRef.GetCloneURL(d.opts.AuthMethod.TransportType())

	log.Infof("Starting to clone the repository %s", d.repoRef)
	// Do a clone operation to the temporary directory
	var err error
	d.repo, err = git.PlainCloneContext(ctx, d.Dir(), false, &git.CloneOptions{
		URL:           cloneURL,
		Auth:          d.opts.AuthMethod,
		ReferenceName: plumbing.NewBranchReferenceName(d.opts.Branch),
		SingleBranch:  true,
		NoCheckout:    false,
		//Depth:             1, // ref: https://github.com/src-d/go-git/issues/1143
		RecurseSubmodules: 0,
		Progress:          nil,
		Tags:              git.NoTags,
	})
	// Handle errors
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git clone operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return fmt.Errorf("git clone was cancelled: %w", err)
	} else if err != nil {
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

	log.Infof("Repo cloned; HEAD commit is %s", ref.Hash())
	return nil
}

func (d *LocalClone) Pull(ctx context.Context) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// TODO: This should support doing Fetch() only maybe
	// TODO: Remove the requirement to actually be on the branch
	// that is being pulled.

	// Make sure it's okay to read
	if err := d.verifyRead(); err != nil {
		return err
	}

	// Perform the git pull operation. The context carries a timeout
	log.Trace("Starting pull operation")
	err := d.wt.PullContext(ctx, &git.PullOptions{
		Auth:         d.opts.AuthMethod,
		SingleBranch: true,
	})

	// Handle errors
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		// all good, nothing more to do
		log.Trace("Pull already up-to-date")
		return nil
	} else if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git pull operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return fmt.Errorf("git pull was cancelled: %w", err)
	} else if err != nil {
		return fmt.Errorf("git pull error: %v", err)
	}

	log.Trace("Pulled successfully")

	// Get current HEAD
	ref, err := d.repo.Head()
	if err != nil {
		return err
	}

	log.Infof("New commit observed %s", ref.Hash())
	return nil
}

func (d *LocalClone) Push(ctx context.Context) error {
	// TODO: Push a specific branch only. Use opts.RefSpecs?

	// Perform the git push operation. The context carries a timeout
	log.Debug("Starting push operation")
	err := d.repo.PushContext(ctx, &git.PushOptions{
		Auth: d.opts.AuthMethod,
	})

	// Handle errors
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		// TODO: Is it good if there's nothing more to do; or a failure if there's nothing to push?
		log.Trace("Push already up-to-date")
		return nil
	} else if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git push operation timed out: %w", err)
	} else if errors.Is(err, context.Canceled) {
		return fmt.Errorf("git push was cancelled: %w", err)
	} else if err != nil {
		return fmt.Errorf("git push error: %v", err)
	}

	log.Trace("Pushed successfully")

	return nil
}

func (d *LocalClone) CreateBranch(_ context.Context, branch string) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// TODO: Should the caller do a force-reset using ResetToCleanBranch before creating the branch?

	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	return d.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
		Create: true,
	})
}

func (d *LocalClone) ResetToCleanBranch(_ context.Context, branch string) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

	// Make sure it's okay to write
	if err := d.verifyWrite(); err != nil {
		return err
	}

	// Best-effort clean
	_ = d.wt.Clean(&git.CleanOptions{
		Dir: true,
	})
	// Force-checkout the main branch
	// TODO: If a transaction (non-branched) was able to commit, and failed after that
	// we need to roll back that commit.
	return d.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
		Force:  true,
	})
	// TODO: Do a pull here too?
}

// Commit creates a commit of all changes in the current worktree with the given parameters.
// It also automatically pushes the branch after the commit.
// ErrNotStarted is returned if the repo hasn't been cloned yet.
// ErrCannotWriteToReadOnly is returned if opts.AuthMethod wasn't provided.
func (d *LocalClone) Commit(ctx context.Context, commit transactional.Commit) error {
	// Lock the mutex now that we're starting, and unlock it when exiting
	d.lock.Lock()
	defer d.lock.Unlock()

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
		// TODO: Should this be an error instead?
		return nil
	}

	// Do a commit
	log.Debug("Committing all local changes")
	hash, err := d.wt.Commit(commit.GetMessage().String(), &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  commit.GetAuthor().GetName(),
			Email: commit.GetAuthor().GetEmail(),
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("git commit error: %v", err)
	}

	// Notify upstream that we now have a new commit, and allow writing again
	log.Infof("A new commit has been created: %q", hash)
	return nil
}

// Cleanup cancels running goroutines and operations, and removes the temporary clone directory
func (d *LocalClone) Cleanup() error {
	// Remove the temporary directory
	if err := os.RemoveAll(d.Dir()); err != nil {
		log.Errorf("Failed to clean up temp git directory: %v", err)
		return err
	}
	return nil
}
