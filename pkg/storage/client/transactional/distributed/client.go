package distributed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/commit"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/util/wait"
)

// NewClient creates a new distributed Client using the given underlying transactional Client,
// remote, and options that configure how the Client should respond to network partitions.
func NewClient(c transactional.Client, remote Remote, opts ...ClientOption) (Client, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: c is mandatory", core.ErrInvalidParameter)
	}
	if remote == nil {
		return nil, fmt.Errorf("%w: remote is mandatory", core.ErrInvalidParameter)
	}

	o := defaultOptions().ApplyOptions(opts)

	g := &generic{
		GenericClient: c,
		remote:        remote,
		opts:          *o,
		branchLocks:   make(map[string]*branchLock),
		branchLocksMu: &sync.Mutex{},
	}

	// Construct the default client
	dc := &genericWithRef{g, nil, commit.Default()}

	// Register ourselves to hook into the transactional.Client's operations
	c.CommitHookChain().Register(dc)
	c.TransactionHookChain().Register(dc)

	return dc, nil
}

type generic struct {
	transactional.GenericClient
	remote Remote
	opts   ClientOptions
	// branchLocks maps a given branch to a given lock the state of the branch
	branchLocks map[string]*branchLock
	// branchLocksMu guards branchLocks
	branchLocksMu *sync.Mutex
}

type genericWithRef struct {
	*generic
	hash commit.Hash
	ref  commit.Ref
}

func (c *genericWithRef) AtHash(h commit.Hash) Client {
	return &genericWithRef{generic: c.generic, hash: h, ref: c.ref}
}
func (c *genericWithRef) AtRef(symbolic commit.Ref) Client {
	// TODO: Invalid (programmer error) to pass symbolic == nil
	return &genericWithRef{generic: c.generic, hash: c.hash, ref: symbolic}
}
func (c *genericWithRef) CurrentRef() commit.Ref {
	return c.ref
}
func (c *genericWithRef) CurrentHash() (commit.Hash, error) {
	// Use the fixed hash if set
	if c.hash != nil {
		return c.hash, nil
	}
	// Otherwise, lookup the symbolic
	return c.ref.Resolve(c.TransactionManager().RefResolver())
}

type branchLockKeyImpl struct{}

var branchLockKey = branchLockKeyImpl{}

type branchLock struct {
	// mu should be write-locked whenever the branch is actively running any
	// function from the remote
	mu *sync.RWMutex
	// lastPull is guarded by mu, before reading, one should RLock mu
	lastPull time.Time
}

/*
	TxMode.AllowReads is incompatible with the PC/EC distributed mode, and might be with the PC/EL mode.
	Ahh, just completely remove the AllowReads mode. If the person wants to do a read for a specific version
	while a tx is going on for a branch, they just need to specify the direct commit.
*/

func (c *genericWithRef) Get(ctx context.Context, key core.ObjectKey, obj client.Object) error {
	return c.readWhenPossible(ctx, func(ctx context.Context) error {
		return c.GenericClient.Get(ctx, key, obj)
	})
}

func (c *genericWithRef) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.readWhenPossible(ctx, func(ctx context.Context) error {
		return c.GenericClient.List(ctx, list, opts...)
	})
}

func (c *genericWithRef) readWhenPossible(ctx context.Context, operation func(context.Context) error) error {
	// If the read is immutable, just proceed
	if _, ok := commit.GetHash(ctx); ok {
		return operation(ctx)
	}
	if c.hash != nil {
		return operation(commit.WithHash(ctx, c.hash))
	}

	// Use the ref from the context, if set, otherwise default to the one configured
	// in this Client.
	ref, ok := commit.GetRef(ctx)
	if !ok {
		ref = c.ref
	}

	// If the read is reference-based; look it up if it needs resync first
	if c.needsResync(ref) {
		// Try to pull the remote ref. If it fails, use returnErr to figure out if
		// this (depending on the configured PACELC mode) is a critical error, or if we
		// should continue with the read
		if err := c.pull(ctx, ref); err != nil {
			if criticalErr := c.returnErr(err); criticalErr != nil {
				return criticalErr
			}
		}
	}
	// Do the read operation
	return operation(commit.WithRef(ctx, ref))
}

// makes a string representation of the ref that is used to uniquely determine
// if two refs are "similar" (i.e. are touching the same resource to be pulled)
func refToStr(ref commit.Ref) string {
	return fmt.Sprintf("%s-%s", ref.Type(), ref.Target())
}

func (c *genericWithRef) getBranchLockInfo(ref commit.Ref) *branchLock {
	c.branchLocksMu.Lock()
	defer c.branchLocksMu.Unlock()

	// Check if there exists a lock for that ref
	str := refToStr(ref)
	info, ok := c.branchLocks[str]
	if ok {
		return info
	}
	// Write to the branchLocks map
	c.branchLocks[str] = &branchLock{
		mu: &sync.RWMutex{},
	}
	return c.branchLocks[str]
}

func (c *genericWithRef) needsResync(ref commit.Ref) bool {
	// Always resync if the cache is always directly invalidated
	cacheValid := c.opts.CacheValidDuration
	if cacheValid == 0 {
		return true
	}

	lck := c.getBranchLockInfo(ref)
	// Lock while reading the last resync time
	lck.mu.RLock()
	defer lck.mu.RUnlock()
	// Resync if there has been no sync so far, or if the last resync was too long ago
	return lck.lastPull.IsZero() || time.Since(lck.lastPull) > cacheValid
}

// StartResyncLoop starts a resync loop for the given branches for
// the given interval.
//
// resyncCacheInterval specifies the interval for which resyncs
// (remote Pulls) should be run in the background. The duration must
// be positive, and non-zero.
//
// resyncBranches specifies what branches to resync. The default is
// []commit.Ref{commit.Default()}, i.e. only the "default" branch.
//
// ctx should be used to cancel the loop, if needed.
//
// While it is technically possible to start many of these resync
// loops, it is not recommended. Start it once, for all the branches
// you need. The branches will be pulled synchronously in order. The
// resync interval is non-sliding, which means that the interval
// includes the time of the operations.
func (c *genericWithRef) StartResyncLoop(ctx context.Context, resyncCacheInterval time.Duration, sync ...commit.Ref) {
	log := c.logger(ctx)
	// Only start this loop if resyncCacheInterval > 0
	if resyncCacheInterval <= 0 {
		log.Info("No need to start the resync loop; resyncCacheInterval <= 0")
		return
	}
	// If unset, only sync the default branch.
	if sync == nil {
		sync = []commit.Ref{commit.Default()}
	}

	// Start the resync goroutine
	go c.resyncLoop(ctx, resyncCacheInterval, sync)
}

func (c *genericWithRef) logger(ctx context.Context) logr.Logger {
	return logr.FromContextOrDiscard(ctx).WithName("distributed.Client")
}

func (c *genericWithRef) resyncLoop(ctx context.Context, resyncCacheInterval time.Duration, sync []commit.Ref) {
	log := c.logger(ctx).WithName("resyncLoop")
	log.V(2).Info("starting resync loop")

	wait.NonSlidingUntilWithContext(ctx, func(_ context.Context) {

		for _, branch := range sync {
			log.V(2).Info("resyncLoop: Will perform pull operation on branch: %q", branch)
			// Perform a fetch, pull & checkout of the new revision
			if err := c.pull(ctx, branch); err != nil {
				log.Error(err, "remote pull failed")
				return
			}
		}
	}, resyncCacheInterval)
	log.V(2).Info("context cancelled, exiting resync loop")
}

func (c *genericWithRef) pull(ctx context.Context, ref commit.Ref) error {
	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(ref)
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Create a new context that times out after the given duration
	ctx, cancel := context.WithTimeout(ctx, c.opts.PullTimeout)
	defer cancel()

	// Make a ctx with the given ref
	ctx = commit.WithRef(ctx, ref)
	if err := c.remote.Pull(ctx); err != nil {
		return err
	}

	// Register the timestamp into the lock
	lck.lastPull = time.Now()
	return nil
}

func (c *genericWithRef) PreTransactionHook(ctx context.Context, info transactional.TxInfo) error {
	// We count on ctx having the VersionRef registered for the head branch

	// Always Pull the _base_ branch before a transaction, to be up-to-date
	// before creating the new head branch
	ref := commit.AtBranch(info.Target.DestBranch())
	if err := c.pull(ctx, ref); err != nil {
		// TODO: Consider a wrapping closure here instead of having to remember to
		// wrap the error in returnErr
		return c.returnErr(err)
	}

	return nil
}

func (c *genericWithRef) PreCommitHook(context.Context, transactional.TxInfo, commit.Request) error {
	return nil // nothing to do here
}

func (c *genericWithRef) PostCommitHook(ctx context.Context, info transactional.TxInfo, _ commit.Request) error {
	// Push the branch in the ctx
	ref := commit.AtBranch(info.Target.DestBranch())
	if err := c.push(ctx, ref); err != nil {
		return c.returnErr(err)
	}
	return nil
}

func (c *genericWithRef) PostTransactionHook(context.Context, transactional.TxInfo) error {
	return nil // nothing to do here; if we had locking capability one would unlock
}

func (c *genericWithRef) Remote() Remote { return c.remote }

func (c *genericWithRef) returnErr(err error) error {
	// If RemoteErrorStream isn't defined, just pass the error through
	if c.opts.RemoteErrorStream == nil {
		return err
	}
	// Non-blocking send to the channel, and no return error
	go func() {
		c.opts.RemoteErrorStream <- err
	}()
	return nil
}

func (c *genericWithRef) push(ctx context.Context, ref commit.Ref) error {
	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(ref)
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Create a new context that times out after the given duration
	ctx, cancel := context.WithTimeout(ctx, c.opts.PushTimeout)
	defer cancel()

	// Push the head branch using the remote
	// If the Push fails, don't execute any other later statements
	return c.remote.Push(ctx)
}

/*

func (c *genericWithRef) branchFromCtx(ctx context.Context) string {
	return core.GetVersionRef(ctx).Branch()
}

// Lock the branch for writing, if supported by the remote
	// If the lock fails, we DO NOT try to pull, but just exit (either with err or a nil error,
	// depending on the configured PACELC mode)
	// TODO: Can we rely on the timeout being exact enough here?
	// TODO: How to do this before the branch even exists...?
	if err := c.lock(ctx, info.Options.Timeout); err != nil {
		return c.returnErr(err)
	}

// Unlock the head branch, if supported
	if err := c.unlock(ctx); err != nil {
		return c.returnErr(err)
	}

func (c *genericWithRef) lock(ctx context.Context, d time.Duration) error {
	lr, ok := c.remote.(LockableRemote)
	if !ok {
		return nil
	}

	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(c.branchFromCtx(ctx))
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Enforce a timeout
	lockCtx, cancel := context.WithTimeout(ctx, c.opts.LockTimeout)
	defer cancel()

	return lr.Lock(lockCtx, d)
}

func (c *genericWithRef) unlock(ctx context.Context) error {
	lr, ok := c.remote.(LockableRemote)
	if !ok {
		return nil
	}

	// Need to get the branch-specific lock variable
	lck := c.getBranchLockInfo(c.branchFromCtx(ctx))
	// Write-lock while this operation is in progress
	lck.mu.Lock()
	defer lck.mu.Unlock()

	// Enforce a timeout
	unlockCtx, cancel := context.WithTimeout(ctx, c.opts.LockTimeout)
	defer cancel()

	return lr.Unlock(unlockCtx)
}
*/
