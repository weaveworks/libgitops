package transactional

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/commit"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

type Client interface {
	GenericClient

	AtHash(commit.Hash) Client
	AtRef(commit.Ref) Client
}

type GenericClient interface {
	client.Reader

	CurrentHash() (commit.Hash, error)
	CurrentRef() commit.Ref

	TransactionManager() TransactionManager
	// KeyedLock is used for locking operations targeting branches
	//KeyedLock() syncutil.NamedLockMap

	// BranchMerger is optional.
	//BranchMerger() BranchMerger

	// CommitHookChain is a chain of hooks that are run before and after a commit is made.
	CommitHookChain() CommitHookChain
	// TransactionHookChain is a chain of hooks that are run before and after a transaction.
	TransactionHookChain() TransactionHookChain

	// Transaction creates a new transaction on the branch stored in the context, so that
	// no other writes to that branch can take place meanwhile.
	//Transaction(ctx context.Context, opts ...TxOption) Tx

	// Transaction creates a new "head" branch (if branchName) with the given {branchName} name, based
	// on the "base" branch in the context. The "base" branch is not locked for writing while
	// the transaction is running, but the head branch is.
	Transaction(ctx context.Context, branchName string, opts ...TxOption) Tx
}

type TransactionManager interface {
	// Init is run at the beginning of the transaction
	Init(ctx context.Context, tx *TxInfo) error

	// Commit creates a new commit for the given branch.
	//
	Commit(ctx context.Context, tx *TxInfo, req commit.Request) error

	Abort(ctx context.Context, tx *TxInfo) error

	//RefResolver() commit.RefResolver
	//CommitResolver() commit.Resolver

	// CreateBranch creates a new branch with the given target branch name. It forks out
	// of the branch specified in the context.
	//CreateBranch(ctx context.Context, branch string) error
	// ResetToCleanVersion switches back to the given branch; but first discards all non-committed
	// changes.
	//ResetToCleanVersion(ctx context.Context, ref core.VersionRef) error

	/*// LockVersionRef takes the VersionRef attached in the context, and makes sure that it is
	// "locked" to the current commit for a given branch.
	LockVersionRef(ctx context.Context) (context.Context, error)*/
}

/*type BranchMerger interface {
	MergeBranches(ctx context.Context, base, head core.VersionRef, commit Commit) error
}*/

type CustomTxFunc func(ctx context.Context) error

type Tx interface {
	Commit(req commit.Request) error
	Abort(err error) error

	Client() client.Client

	// TODO: Rename to Do/Run/Execute
	Custom(CustomTxFunc) Tx

	Get(key core.ObjectKey, obj client.Object) Tx
	List(list client.ObjectList, opts ...client.ListOption) Tx

	Create(obj client.Object, opts ...client.CreateOption) Tx
	Update(obj client.Object, opts ...client.UpdateOption) Tx
	Patch(obj client.Object, patch client.Patch, opts ...client.PatchOption) Tx
	Delete(obj client.Object, opts ...client.DeleteOption) Tx
	DeleteAllOf(obj client.Object, opts ...client.DeleteAllOfOption) Tx

	UpdateStatus(obj client.Object, opts ...client.UpdateOption) Tx
	PatchStatus(obj client.Object, patch client.Patch, opts ...client.PatchOption) Tx
}

/*type BranchTx interface {
	CreateTx(Commit) BranchTxResult
	Abort(err error) error

	Client() client.Client

	Custom(CustomTxFunc) BranchTx

	Get(key core.ObjectKey, obj client.Object) BranchTx
	List(list client.ObjectList, opts ...client.ListOption) BranchTx

	Create(obj client.Object, opts ...client.CreateOption) BranchTx
	Update(obj client.Object, opts ...client.UpdateOption) BranchTx
	Patch(obj client.Object, patch client.Patch, opts ...client.PatchOption) BranchTx
	Delete(obj client.Object, opts ...client.DeleteOption) BranchTx
	DeleteAllOf(obj client.Object, opts ...client.DeleteAllOfOption) BranchTx

	UpdateStatus(obj client.Object, opts ...client.UpdateOption) BranchTx
	PatchStatus(obj client.Object, patch client.Patch, opts ...client.PatchOption) BranchTx
}

type BranchTxResult interface {
	Error() error
	MergeWithBase(Commit) error
}*/
