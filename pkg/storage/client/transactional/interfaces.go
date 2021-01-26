package transactional

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

type Client interface {
	client.Reader

	BranchManager() BranchManager
	BranchMerger() BranchMerger

	Transaction(ctx context.Context, opts ...TxOption) Tx
	BranchTransaction(ctx context.Context, branchName string, opts ...TxOption) BranchTx
}

type BranchManager interface {
	CreateBranch(ctx context.Context, branch string) error
	ResetToCleanBranch(ctx context.Context, branch string) error
	Commit(ctx context.Context, commit Commit) error

	CommitHandler() CommitHandler
	TransactionHandler() TransactionHandler
}

type BranchMerger interface {
	MergeBranches(ctx context.Context, base, head string, commit Commit) error
}

type CustomTxFunc func(ctx context.Context) error

type Tx interface {
	Commit(Commit) error
	Abort(err error) error

	Client() client.Client

	Custom(CustomTxFunc) Tx

	Get(key core.ObjectKey, obj core.Object) Tx
	List(list core.ObjectList, opts ...core.ListOption) Tx

	Create(obj core.Object, opts ...core.CreateOption) Tx
	Update(obj core.Object, opts ...core.UpdateOption) Tx
	Patch(obj core.Object, patch core.Patch, opts ...core.PatchOption) Tx
	Delete(obj core.Object, opts ...core.DeleteOption) Tx
	DeleteAllOf(obj core.Object, opts ...core.DeleteAllOfOption) Tx

	UpdateStatus(obj core.Object, opts ...core.UpdateOption) Tx
	PatchStatus(obj core.Object, patch core.Patch, opts ...core.PatchOption) Tx
}

type BranchTx interface {
	CreateTx(Commit) BranchTxResult
	Abort(err error) error

	Client() client.Client

	Custom(CustomTxFunc) BranchTx

	Get(key core.ObjectKey, obj core.Object) BranchTx
	List(list core.ObjectList, opts ...core.ListOption) BranchTx

	Create(obj core.Object, opts ...core.CreateOption) BranchTx
	Update(obj core.Object, opts ...core.UpdateOption) BranchTx
	Patch(obj core.Object, patch core.Patch, opts ...core.PatchOption) BranchTx
	Delete(obj core.Object, opts ...core.DeleteOption) BranchTx
	DeleteAllOf(obj core.Object, opts ...core.DeleteAllOfOption) BranchTx

	UpdateStatus(obj core.Object, opts ...core.UpdateOption) BranchTx
	PatchStatus(obj core.Object, patch core.Patch, opts ...core.PatchOption) BranchTx
}

type BranchTxResult interface {
	Error() error
	MergeWithBase(Commit) error
}
