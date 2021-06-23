package transactional

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	syncutil "github.com/weaveworks/libgitops/pkg/util/sync"
	utilerrs "k8s.io/apimachinery/pkg/util/errors"
)

var _ Client = &Generic{}

func NewGeneric(c client.Client, manager TransactionManager) (Client, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: c is required", core.ErrInvalidParameter)
	}
	if manager == nil {
		return nil, fmt.Errorf("%w: manager is required", core.ErrInvalidParameter)
	}
	g := &Generic{
		c:           c,
		lockMap:     syncutil.NewNamedLockMap(),
		txHooks:     &MultiTransactionHook{},
		commitHooks: &MultiCommitHook{},
		manager:     manager,
		//merger:      merger,
	}
	// We must be able to resolve versions
	if g.versionRefResolver() == nil {
		return nil, fmt.Errorf("%w: the underlying Client must provide a VersionRefResolver through its Storage", core.ErrInvalidParameter)
	}
	return g, nil
}

type Generic struct {
	c client.Client

	lockMap syncutil.NamedLockMap

	// Hooks
	txHooks     TransactionHookChain
	commitHooks CommitHookChain

	// +required
	manager TransactionManager
}

type txLockKeyImpl struct{}

var txLockKey = txLockKeyImpl{}

type txLock struct {
	// mode specifies what transaction mode is used; Atomic or AllowReading.
	//mode TxMode
	// active == 1 means "transaction active, mu is locked for writing"
	// active == 0 means "transaction has stopped, mu has been unlocked"
	active uint32
}

func (c *Generic) Get(ctx context.Context, key core.ObjectKey, obj client.Object) error {
	return c.lockAndRead(ctx, func() error {
		return c.c.Get(ctx, key, obj)
	})
}

func (c *Generic) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.lockAndRead(ctx, func() error {
		return c.c.List(ctx, list, opts...)
	})
}

func (c *Generic) versionRefResolver() core.VersionRefResolver {
	return c.c.BackendReader().Storage().VersionRefResolver()
}

func (c *Generic) lockForBranch(branch string) (syncutil.LockWithData, *txLock, bool) {
	lck := c.lockMap.LockByName(branch)
	txState, ok := lck.QLoad(txLockKey).(*txLock)
	return lck, txState, ok
}

func (c *Generic) lockAndRead(ctx context.Context, callback func() error) error {
	ref := core.GetVersionRef(ctx)

	_, immutable, err := c.versionRefResolver().ResolveVersionRef(ref)
	if err != nil {
		return err
	} else if immutable {
		// If this is an immutable revision, just continue the call
		return callback()
	}

	// At this point we know that ref is mutable (what we call a "branch" here), and commit is the fixed revision
	lck := c.lockMap.LockByName(ref)
	lck.Lock()
	defer lck.Unlock()
	// TODO: At what point should we resolve the "branch" -> "commit" part? Should we expect that to be done in the
	// filesystem only?
	return callback()
}

func (c *Generic) initTx(ctx context.Context, info TxInfo) (context.Context, txFunc) {
	// Get the head branch lock and status
	lck := c.lockMap.LockByName(info.HeadBranch)

	// Wait for all reads to complete (in the case of the atomic more),
	// and then lock for writing. For non-atomic mode this uses the mutex
	// as it is modifying txState, and two transactions must not run at
	// the same time for the same branch.
	//
	// Always lock mu when a transaction is running on this branch,
	// regardless of mode. If atomic mode is enabled, this also waits
	// on any reads happening at this moment. For all modes, this ensures
	// transactions happen in order.
	lck.Lock()
	txState := &txLock{
		active: 1, // set tx state to "active"
		//mode:   info.Options.Mode, // declare what transaction mode is used
	}
	lck.Store(txLockKey, txState)

	// Create a child context with a timeout
	dlCtx, cleanupTimeout := context.WithTimeout(ctx, info.Options.Timeout)

	// This function cleans up the transaction, and unlocks the tx muted
	cleanupFunc := func() error {
		// Cleanup after the transaction
		if err := c.cleanupAfterTx(ctx, &info); err != nil {
			return fmt.Errorf("Failed to cleanup branch %s after tx: %v", info.HeadBranch, err)
		}
		// Unlock the mutex so new transactions can take place on this branch
		lck.Unlock()
		return nil
	}

	// Start waiting for the cancellation of the deadline context.
	go func() {
		// Wait for the context to either timeout or be cancelled
		<-dlCtx.Done()
		// This guard makes sure the cleanup function runs exactly
		// once, regardless of transaction end cause.
		if atomic.CompareAndSwapUint32(&txState.active, 1, 0) {
			if err := cleanupFunc(); err != nil {
				logrus.Errorf("Failed to cleanup after tx timeout: %v", err)
			}
		}
	}()

	abortFunc := func() error {
		// The transaction ended; the caller is either Abort() or
		// at the end of a successful transaction. The cause of
		// Abort() happening can also be a context cancellation.
		// If the parent context was cancelled or timed out; this
		// function and the above function race to set active => 0
		// Regardless, due to the atomic nature of the operation,
		// cleanupFunc() will only be run once.
		if atomic.CompareAndSwapUint32(&txState.active, 1, 0) {
			// We can now stop the timeout timer
			cleanupTimeout()
			// Clean up the transaction
			return cleanupFunc()
		}
		return nil
	}

	return dlCtx, abortFunc
}

func (c *Generic) cleanupAfterTx(ctx context.Context, info *TxInfo) error {
	// Always both clean the branch, and run post-tx tasks
	return utilerrs.NewAggregate([]error{
		// TODO: This should be "clean up the writable area"
		c.manager.ResetToCleanVersion(ctx, info.Base),
		// TODO: should this be in its own goroutine to switch back to main
		// ASAP?
		c.TransactionHookChain().PostTransactionHook(ctx, *info),
	})
}

func (c *Generic) BackendReader() backend.Reader {
	return c.c.BackendReader()
}

/*func (c *Generic) BranchMerger() BranchMerger {
	return c.merger
}*/

func (c *Generic) TransactionManager() TransactionManager {
	return c.manager
}

func (c *Generic) TransactionHookChain() TransactionHookChain {
	return c.txHooks
}

func (c *Generic) CommitHookChain() CommitHookChain {
	return c.commitHooks
}

func (c *Generic) Transaction(ctx context.Context, opts ...TxOption) Tx {
	tx, err := c.transaction(ctx, opts...)
	if err != nil {
		// TODO: Return a Tx with an error included
		panic(err)
	}
	return tx
}

func (c *Generic) BranchTransaction(ctx context.Context, headBranch string, opts ...TxOption) Tx {
	tx, err := c.branchTransaction(ctx, headBranch, opts...)
	if err != nil {
		// TODO: Return a Tx with an error included
		panic(err)
	}
	return tx
}

var ErrVersionRefIsImmutable = errors.New("cannot execute transaction against immutable version ref")

func (c *Generic) transaction(ctx context.Context, opts ...TxOption) (Tx, error) {
	// Rules: A transaction executes against "itself".

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	ref := core.GetVersionRef(ctx)

	baseCommit, isImmutable, err := c.versionRefResolver().ResolveVersionRef(ref)
	if err != nil {
		return nil, err
	}
	// We cannot apply a transaction against an immutable version
	if isImmutable {
		return nil, fmt.Errorf("%w: %s", ErrVersionRefIsImmutable, ref)
	}

	info := TxInfo{
		BaseCommit: baseCommit,
		HeadBranch: ref,
		Options:    *o,
	}
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc := c.initTx(ctx, info)

	// Run pre-tx checks
	if err := c.TransactionHookChain().PreTransactionHook(ctxWithDeadline, info); err != nil {
		return nil, err
	}

	return &txImpl{
		&txCommon{
			c:           c.c,
			manager:     c.manager,
			commitHook:  c.CommitHookChain(),
			ctx:         ctxWithDeadline,
			info:        info,
			cleanupFunc: cleanupFunc,
		},
	}, nil
}

func (c *Generic) branchTransaction(ctx context.Context, headBranch string, opts ...TxOption) (Tx, error) {
	// Get the base version reference. It is ok if it's immutable, too.
	baseRef := core.GetVersionRef(ctx)

	// Append random bytes to the end of the head branch if it ends with a dash
	if strings.HasSuffix(headBranch, "-") {
		suffix, err := randomSHA(4)
		if err != nil {
			return nil, err
		}
		headBranch += suffix
	}

	// Validate that the base and head branches are distinct
	if baseRef == headBranch {
		return nil, fmt.Errorf("head and target branches must not be the same")
	}

	logrus.Debugf("Base VersionRef: %q. Head branch: %q.", baseRef, headBranch)

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	// Resolve what the base commit is
	baseCommit, _, err := c.versionRefResolver().ResolveVersionRef(baseRef)
	if err != nil {
		return nil, err
	}

	info := TxInfo{
		BaseCommit: baseCommit,
		HeadBranch: headBranch,
		Options:    *o,
	}

	// Register the head branch with the context
	// TODO: We should register all of TxInfo here instead, or ...?
	ctxWithHeadBranch := core.WithVersionRef(ctx, headBranch)
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc := c.initTx(ctxWithHeadBranch, info)

	// Run pre-tx checks and create the new branch
	if err := utilerrs.NewAggregate([]error{
		c.TransactionHookChain().PreTransactionHook(ctxWithDeadline, info),
		c.manager.CreateBranch(ctxWithDeadline, headBranch),
	}); err != nil {
		return nil, err
	}

	return &txImpl{
		txCommon: &txCommon{
			c:           c.c,
			manager:     c.manager,
			commitHook:  c.CommitHookChain(),
			ctx:         ctxWithDeadline,
			info:        info,
			cleanupFunc: cleanupFunc,
		},
	}, nil
}
