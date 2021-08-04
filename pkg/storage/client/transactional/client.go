package transactional

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/weaveworks/libgitops/pkg/storage/backend"
	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/commit"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"go.uber.org/atomic"
	"k8s.io/apimachinery/pkg/types"
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
		txs:         make(map[types.UID]*atomic.Bool),
		txsMu:       &sync.Mutex{},
	}
	return &genericWithRef{g, nil, commit.Default()}, nil
}

type generic struct {
	c client.Client

	//lockMap syncutil.NamedLockMap

	// Hooks
	txHooks     TransactionHookChain
	commitHooks CommitHookChain

	// +required
	manager TransactionManager

	txs   map[types.UID]*atomic.Bool
	txsMu *sync.Mutex
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
	return c.ref.Resolve(c.manager.RefResolver())
}

func (c *genericWithRef) Get(ctx context.Context, key core.ObjectKey, obj client.Object) error {
	return c.defaultCtxCommitRef(ctx, func(ctx context.Context) error {
		return c.c.Get(ctx, key, obj)
	})
}

func (c *genericWithRef) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.defaultCtxCommitRef(ctx, func(ctx context.Context) error {
		return c.c.List(ctx, list, opts...)
	})
}

// defaultCtxCommitRef makes sure that there's either commit.Hash registered with the context when reading
// TODO: In the future, shall filesystems also support commit.Ref?
func (c *genericWithRef) defaultCtxCommitRef(ctx context.Context, callback func(ctx context.Context) error) error {
	// If ctx already specifies an immutable version to read, use it
	if _, ok := commit.GetHash(ctx); ok {
		return callback(ctx)
	}
	// If ctx specifies a symbolic target, resolve it
	if ref, ok := commit.GetRef(ctx); ok {
		h, err := ref.Resolve(c.manager.RefResolver())
		if err != nil {
			return err
		}
		return callback(commit.WithHash(ctx, h))
	}

	// Otherwise, look it up based on this client's data
	h, err := c.CurrentHash()
	if err != nil {
		return err
	}

	// TODO: At what point should we resolve the "branch" -> "commit" part? Should we expect that to be done in the
	// filesystem only?
	return callback(commit.WithHash(ctx, h))
}

func (c *genericWithRef) txStateByUID(uid types.UID) *atomic.Bool {
	// c.txsMu guards reads and writes of the c.txs map
	c.txsMu.Lock()
	defer c.txsMu.Unlock()

	// Check if information about a transaction on this branch exists.
	state, ok := c.txs[uid]
	if ok {
		return state
	}
	// if not, grow the txs map by one and return it
	c.txs[uid] = atomic.NewBool(false)
	return c.txs[uid]
}
func (c *genericWithRef) initTx(ctx context.Context, info TxInfo) (context.Context, txFunc, error) {
	log := logr.FromContextOrDiscard(ctx)

	active := c.txStateByUID(info.Target.UUID())
	// If active == false, then this will switch active => true and return true
	// If active == true, then no operation will take place, and false is returned
	// In other words, if false is returned, a transaction with this UID is ongoing.
	// However, a UID conflict is very unlikely, given randomness and length of the UID
	if !active.CAS(false, true) {
		// TODO: Avoid this possibility
		return nil, nil, errors.New("should never happen; UID conflict")
	}

	// Create a child context with a timeout
	dlCtx, cleanupTimeout := context.WithTimeout(ctx, info.Options.Timeout)

	// This function cleans up the transaction, and unlocks the tx muted
	cleanupFunc := func() error {
		// Cleanup after the transaction
		if err := c.cleanupAfterTx(ctx, &info); err != nil {
			return fmt.Errorf("Failed to cleanup branch %s after tx: %v", info.Target.DestBranch(), err)
		}
		// Avoid leaking memory by growing c.txs infinitely
		c.txsMu.Lock()
		delete(c.txs, info.Target.UUID())
		c.txsMu.Unlock()
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
				log.Error(err, "failed to cleanup after tx timeout")
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
	log := logr.FromContextOrDiscard(ctx)

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

	log.V(2).Info("Base commit hash: %q. Head branch: %q.", baseHash, headBranch)

	// Parse options
	o := defaultTxOptions().ApplyOptions(opts)

	target := commit.NewMutableTarget(headBranch, baseHash)
	info := TxInfo{
		Target:  target,
		Options: *o,
	}

	// Register the head branch with the context
	// TODO: We should register all of TxInfo here instead, or ...?
	ctxWithDestBranch := commit.WithMutableTarget(ctx, target)
	// Initialize the transaction
	ctxWithDeadline, cleanupFunc, err := c.initTx(ctxWithDestBranch, info)
	if err != nil {
		return nil, err
	}

	// Run pre-tx checks and create the new branch
	// TODO: Use uber's multierr?
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
