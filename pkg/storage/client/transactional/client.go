package transactional

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional/commit"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"go.uber.org/atomic"
	utilerrs "k8s.io/apimachinery/pkg/util/errors"
)

var _ Client = &genericWithRef{}

func NewGeneric(c client.Client, manager TransactionManager) (Client, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: c is required", core.ErrInvalidParameter)
	}
	if manager == nil {
		return nil, fmt.Errorf("%w: manager is required", core.ErrInvalidParameter)
	}
	g := &generic{
		c: c,
		//lockMap:     syncutil.NewNamedLockMap(),
		txHooks:     &MultiTransactionHook{},
		commitHooks: &MultiCommitHook{},
		manager:     manager,
		txs:         make(map[string]*atomic.Bool),
		txsMu:       &sync.Mutex{},
	}
	return &genericWithRef{g, commit.Default()}, nil
}

type generic struct {
	c client.Client

	//lockMap syncutil.NamedLockMap

	// Hooks
	txHooks     TransactionHookChain
	commitHooks CommitHookChain

	// +required
	manager TransactionManager

	txs   map[string]*atomic.Bool
	txsMu *sync.Mutex
}

type genericWithRef struct {
	*generic
	ref commit.Ref
}

func (c *genericWithRef) AtRef(ref commit.Ref) Client {
	return &genericWithRef{c.generic, ref}
}
func (c *genericWithRef) AtSymbolicRef(symbolic string) Client {
	return c.AtRef(commit.At(symbolic))
}
func (c *genericWithRef) CurrentRef() commit.Ref {
	return c.ref
}

/*
type txLockKeyImpl struct{}

var txLockKey = txLockKeyImpl{}

type txLock struct {
	// mode specifies what transaction mode is used; Atomic or AllowReading.
	//mode TxMode
	// active == 1 means "transaction active, mu is locked for writing"
	// active == 0 means "transaction has stopped, mu has been unlocked"
	//active uint32
	active *atomic.Bool
}*/

func (c *genericWithRef) Get(ctx context.Context, key core.ObjectKey, obj client.Object) error {
	return c.lockAndRead(ctx, func(ctx context.Context) error {
		return c.c.Get(ctx, key, obj)
	})
}

func (c *genericWithRef) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.lockAndRead(ctx, func(ctx context.Context) error {
		return c.c.List(ctx, list, opts...)
	})
}

/*func (c *genericWithRef) lockForBranch(branch string) (syncutil.LockWithData, *txLock, bool) {
	lck := c.lockMap.LockByName(branch)
	txState, ok := lck.QLoad(txLockKey).(*txLock)
	return lck, txState, ok
}*/

func (c *genericWithRef) lockAndRead(ctx context.Context, callback func(ctx context.Context) error) error {
	h, err := c.ref.Resolve(c.manager.RefResolver())
	if err != nil {
		return err
	}

	// TODO: At what point should we resolve the "branch" -> "commit" part? Should we expect that to be done in the
	// filesystem only?
	return callback(commit.WithHash(ctx, h))
}

func (c *genericWithRef) txStateByName(name string) *atomic.Bool {
	// c.txsMu guards reads and writes of the c.txs map
	c.txsMu.Lock()
	defer c.txsMu.Unlock()

	// Check if information about a transaction on this branch exists.
	state, ok := c.txs[name]
	if ok {
		return state
	}
	// if not, grow the txs map by one and return it
	c.txs[name] = atomic.NewBool(false)
	return c.txs[name]
}
func (c *genericWithRef) initTx(ctx context.Context, info TxInfo) (context.Context, txFunc, error) {
	// Get the head branch lock and status
	//lck := c.lockMap.LockByName(info.HeadBranch)

	// Wait for all reads to complete (in the case of the atomic more),
	// and then lock for writing. For non-atomic mode this uses the mutex
	// as it is modifying txState, and two transactions must not run at
	// the same time for the same branch.
	//
	// Always lock mu when a transaction is running on this branch,
	// regardless of mode. If atomic mode is enabled, this also waits
	// on any reads happening at this moment. For all modes, this ensures
	// transactions happen in order.
	/*lck.Lock()
	txState := &txLock{
		active: 1, // set tx state to "active"
		//mode:   info.Options.Mode, // declare what transaction mode is used
	}
	lck.Store(txLockKey, txState)*/

	active := c.txStateByName(info.HeadBranch)
	// If active == false, then this will switch active => true and return true
	// If active == true, then no operation will take place, and false is returned
	// In other words, if false is returned, a transaction is ongoing and we should
	// return a temporal error
	if !active.CAS(false, true) {
		// TODO: Is this the right way?
		return nil, nil, errors.New("transaction is already ongoing")
	}

	// Create a child context with a timeout
	dlCtx, cleanupTimeout := context.WithTimeout(ctx, info.Options.Timeout)

	// This function cleans up the transaction, and unlocks the tx muted
	cleanupFunc := func() error {
		// Cleanup after the transaction
		if err := c.cleanupAfterTx(ctx, &info); err != nil {
			return fmt.Errorf("Failed to cleanup branch %s after tx: %v", info.HeadBranch, err)
		}
		// Unlock the mutex so new transactions can take place on this branch
		//lck.Unlock()
		return nil
	}

	// Start waiting for the cancellation of the deadline context.
	go func() {
		// Wait for the context to either timeout or be cancelled
		<-dlCtx.Done()
		// This guard makes sure the cleanup function runs exactly
		// once, regardless of transaction end cause.
		if active.CAS(true, false) {
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
		if active.CAS(true, false) {
			// We can now stop the timeout timer
			cleanupTimeout()
			// Clean up the transaction
			return cleanupFunc()
		}
		return nil
	}

	return dlCtx, abortFunc, nil
}

func (c *genericWithRef) cleanupAfterTx(ctx context.Context, info *TxInfo) error {
	// Always both clean the writable area, and run post-tx tasks
	return utilerrs.NewAggregate([]error{
		c.manager.Abort(ctx, info),
		// TODO: should this be in its own goroutine to switch back to main
		// ASAP?
		c.TransactionHookChain().PostTransactionHook(ctx, *info),
	})
}

func (c *genericWithRef) BackendReader() backend.Reader {
	return c.c.BackendReader()
}

func (c *genericWithRef) TransactionManager() TransactionManager {
	return c.manager
}

func (c *genericWithRef) TransactionHookChain() TransactionHookChain {
	return c.txHooks
}

func (c *genericWithRef) CommitHookChain() CommitHookChain {
	return c.commitHooks
}

func (c *genericWithRef) Transaction(ctx context.Context, headBranch string, opts ...TxOption) Tx {
	tx, err := c.transaction(ctx, headBranch, opts...)
	if err != nil {
		// TODO: Return a Tx with an error included
		panic(err)
	}
	return tx
}

var ErrVersionRefIsImmutable = errors.New("cannot execute transaction against immutable version ref")

func (c *genericWithRef) transaction(ctx context.Context, headBranch string, opts ...TxOption) (Tx, error) {
	// Get the immutable base version hash
	baseHash, err := c.ref.Resolve(c.manager.RefResolver())
	if err != nil {
		return nil, err
	}

	// Append random bytes to the end of the head branch if it ends with a dash
	if strings.HasSuffix(headBranch, "-") {
		suffix, err := randomSHA(4)
		if err != nil {
			return nil, err
		}
		headBranch += suffix
	}

	logrus.Debugf("Base commit hash: %q. Head branch: %q.", baseHash, headBranch)

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	info := TxInfo{
		BaseCommit: baseHash,
		HeadBranch: headBranch,
		Options:    *o,
	}

	// Register the head branch with the context
	// TODO: We should register all of TxInfo here instead, or ...?
	ctxWithHeadBranch := commit.WithMutable(ctx, commit.NewMutable(headBranch))
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc, err := c.initTx(ctxWithHeadBranch, info)
	if err != nil {
		return nil, err
	}

	// Run pre-tx checks and create the new branch
	// TODO: Use multierr?
	if err := utilerrs.NewAggregate([]error{
		c.TransactionHookChain().PreTransactionHook(ctxWithDeadline, info),
		c.manager.Init(ctxWithDeadline, &info),
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
