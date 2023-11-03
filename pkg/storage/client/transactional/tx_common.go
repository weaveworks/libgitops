package transactional

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/commit"
	utilerrs "k8s.io/apimachinery/pkg/util/errors"
)

type txFunc func() error

type txCommon struct {
	err         error
	c           client.Client
	manager     TransactionManager
	commitHook  CommitHook
	ctx         context.Context
	ops         []txFunc
	info        TxInfo
	cleanupFunc txFunc
}

func (tx *txCommon) Client() client.Client {
	return tx.c
}

func (tx *txCommon) Abort(err error) error {
	// Run the cleanup function and return an aggregate of the two possible errors
	return utilerrs.NewAggregate([]error{
		err,
		tx.cleanupFunc(),
	})
}

func (tx *txCommon) handlePreCommit(c commit.Request) txFunc {
	return func() error {
		return tx.commitHook.PreCommitHook(tx.ctx, tx.info, c)
	}
}

func (tx *txCommon) commit(c commit.Request) txFunc {
	return func() error {
		return tx.manager.Commit(tx.ctx, &tx.info, c)
	}
}

func (tx *txCommon) handlePostCommit(c commit.Request) txFunc {
	return func() error {
		return tx.commitHook.PostCommitHook(tx.ctx, tx.info, c)
	}
}

func (tx *txCommon) tryApplyAndCommitOperations(c commit.Request) error {
	// If an error occurred already before, just return it directly
	if tx.err != nil {
		return tx.err
	}

	// First, all registered client operations are run
	// Then Pre-commit, commit, and post-commit functions are run
	// If at any stage the context is cancelled, an error is returned
	// immediately, and no more functions in the chain are run. The
	// same goes for errors from any of the functions, the chain is
	// immediately interrupted on errors.
	return execTransactionsCtx(tx.ctx, append(
		tx.ops,
		tx.handlePreCommit(c),
		tx.commit(c),
		tx.handlePostCommit(c),
	))
}
