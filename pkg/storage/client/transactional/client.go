package transactional

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/util"
	utilerrs "k8s.io/apimachinery/pkg/util/errors"
)

var _ Client = &Generic{}

func NewGeneric(c client.Client, manager BranchManager, merger BranchMerger) (Client, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: c is required", core.ErrInvalidParameter)
	}
	if manager == nil {
		return nil, fmt.Errorf("%w: manager is required", core.ErrInvalidParameter)
	}
	return &Generic{
		c:       c,
		txs:     make(map[string]*txLock),
		txsMu:   &sync.Mutex{},
		manager: manager,
		merger:  merger,
	}, nil
}

type Generic struct {
	c client.Client

	txs   map[string]*txLock
	txsMu *sync.Mutex

	// +optional
	merger BranchMerger
	// +required
	manager BranchManager
}

type txLock struct {
	mu   *sync.RWMutex
	mode TxMode
	// active == 1 means "transaction active, mu is locked for writing"
	// active == 0 means "transaction has stopped, mu has been unlocked"
	active uint32
}

func (c *Generic) Get(ctx context.Context, key core.ObjectKey, obj core.Object) error {
	return c.lockForReading(ctx, func() error {
		return c.c.Get(ctx, key, obj)
	})
}

func (c *Generic) List(ctx context.Context, list core.ObjectList, opts ...core.ListOption) error {
	return c.lockForReading(ctx, func() error {
		return c.c.List(ctx, list, opts...)
	})
}

func (c *Generic) lockForReading(ctx context.Context, operation func() error) error {
	ref := core.GetVersionRef(ctx)
	if !ref.IsWritable() {
		// Never block reads for read-only VersionRefs. We know nobody can change
		// them during the read operation, so they should be race condition-free.
		return operation()
	}
	// If the VersionRef is writable; treat it as a branch and lock it to avoid
	// race conditions.
	return c.lockAndReadBranch(ref.String(), operation)
}

func (c *Generic) lockAndReadBranch(branch string, callback func() error) error {
	// Use c.txsMu to guard reads and writes to the c.txs map
	c.txsMu.Lock()
	// Check if information about a transaction on this branch exists.
	txState, ok := c.txs[branch]
	if !ok {
		// grow the txs map by one
		c.txs[branch] = &txLock{
			mu: &sync.RWMutex{},
		}
		txState = c.txs[branch]
	}
	c.txsMu.Unlock()

	// In the atomic mode, we lock the txLock during the read,
	// so no new transactions can be started while the read
	// operation goes on. In non-atomic modes, reads aren't locked,
	// instead it is assumed that downstream implementations just
	// read the latest commit on the given branch.
	if txState.mode == TxModeAtomic {
		txState.mu.RLock()
	}
	err := callback()
	if txState.mode == TxModeAtomic {
		txState.mu.RUnlock()
	}
	return err
}

func (c *Generic) initTx(ctx context.Context, info TxInfo) (context.Context, txFunc) {
	// Aquire the tx-specific lock
	c.txsMu.Lock()
	txState, ok := c.txs[info.Head]
	if !ok {
		// grow the txs map by one
		c.txs[info.Head] = &txLock{
			mu: &sync.RWMutex{},
		}
		txState = c.txs[info.Head]
	}
	txState.mode = info.Options.Mode
	c.txsMu.Unlock()

	// Wait for all reads to complete (in the case of the atomic more),
	// and then lock for writing. For non-atomic mode this uses the mutex
	// as it is modifying txState, and two transactions must not run at
	// the same time for the same branch.
	//
	// Always lock mu when a transaction is running on this branch,
	// regardless of mode. If atomic mode is enabled, this also waits
	// on any reads happening at this moment. For all modes, this ensures
	// transactions happen in order.
	txState.mu.Lock()
	txState.active = 1 // set tx state to "active"

	// Create a child context with a timeout
	dlCtx, cleanupTimeout := context.WithTimeout(ctx, info.Options.Timeout)

	// This function cleans up the transaction, and unlocks the tx muted
	cleanupFunc := func() error {
		// Cleanup after the transaction
		if err := c.cleanupAfterTx(ctx, &info); err != nil {
			return fmt.Errorf("Failed to cleanup branch %s after tx: %v", info.Head, err)
		}
		// Unlock the mutex so new transactions can take place on this branch
		txState.mu.Unlock()
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
		// cleanupFunc() will only be run twice.
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
	// Always run both the "clean branch" command...
	errs := []error{c.manager.ResetToCleanBranch(ctx, info.Base)}
	if c.manager.TransactionHandler() != nil {
		// ... and the post-transaction command
		// TODO: should this be in its own goroutine to switch back to main
		// ASAP?
		errs = append(errs,
			c.manager.TransactionHandler().HandlePostTransaction(ctx, *info))
	}
	// Return an aggregate error
	return utilerrs.NewAggregate(errs)
}

func (c *Generic) BackendReader() backend.Reader {
	return c.c.BackendReader()
}

func (c *Generic) BranchMerger() BranchMerger {
	return c.merger
}

func (c *Generic) BranchManager() BranchManager {
	return c.manager
}

func (c *Generic) Transaction(ctx context.Context, opts ...TxOption) Tx {
	tx, err := c.transaction(ctx, opts...)
	if err != nil {
		panic(err)
	}
	return tx
}

func (c *Generic) BranchTransaction(ctx context.Context, headBranch string, opts ...TxOption) BranchTx {
	tx, err := c.branchTransaction(ctx, headBranch, opts...)
	if err != nil {
		panic(err)
	}
	return tx
}

func (c *Generic) validateCtx(ctx context.Context) (core.VersionRef, error) {
	// Check so versionref is writable
	ref := core.GetVersionRef(ctx)
	if !ref.IsWritable() {
		return nil, fmt.Errorf("must not give a writable VersionRef to (Branch)Transaction()")
	}

	return ref, nil
}

func (c *Generic) transaction(ctx context.Context, opts ...TxOption) (Tx, error) {
	// Validate the versionref from the context
	ref, err := c.validateCtx(ctx)
	if err != nil {
		return nil, err
	}

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	branch := ref.String()
	info := TxInfo{
		Base:    branch,
		Head:    branch,
		Options: *o,
	}
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc := c.initTx(ctx, info)

	// Run pre-tx checks
	err = c.manager.TransactionHandler().HandlePreTransaction(ctxWithDeadline, info)

	return &txImpl{
		&txCommon{
			err:         err,
			c:           c.c,
			manager:     c.manager,
			ctx:         ctxWithDeadline,
			info:        info,
			cleanupFunc: cleanupFunc,
		},
	}, nil
}

func (c *Generic) branchTransaction(ctx context.Context, headBranch string, opts ...TxOption) (BranchTx, error) {
	// Validate the versionref from the context
	ref, err := c.validateCtx(ctx)
	if err != nil {
		return nil, err
	}
	baseBranch := ref.String()

	// Append random bytes to the end of the head branch if it ends with a dash
	if strings.HasSuffix(headBranch, "-") {
		suffix, err := util.RandomSHA(4)
		if err != nil {
			return nil, err
		}
		headBranch += suffix
	}

	// Validate that the base and head branches are distinct
	if baseBranch == headBranch {
		return nil, fmt.Errorf("head and target branches must not be the same")
	}

	logrus.Debugf("Base branch: %q. Head branch: %q.", baseBranch, headBranch)

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	info := TxInfo{
		Base:    baseBranch,
		Head:    headBranch,
		Options: *o,
	}

	// Register the head branch with the context
	ctxWithHeadBranch := core.WithVersionRef(ctx, core.NewBranchRef(headBranch))
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc := c.initTx(ctxWithHeadBranch, info)

	// Run pre-tx checks and create the new branch
	err = utilerrs.NewAggregate([]error{
		c.manager.TransactionHandler().HandlePreTransaction(ctxWithDeadline, info),
		c.manager.CreateBranch(ctxWithDeadline, headBranch),
	})

	return &txBranchImpl{
		txCommon: &txCommon{
			err:         err,
			c:           c.c,
			manager:     c.manager,
			ctx:         ctxWithDeadline,
			info:        info,
			cleanupFunc: cleanupFunc,
		},
		merger: c.merger,
	}, nil
}
