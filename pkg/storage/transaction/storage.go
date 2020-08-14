package transaction

import (
	"context"
	"errors"

	"github.com/weaveworks/libgitops/pkg/storage"
)

var (
	ErrAbortTransaction = errors.New("transaction aborted")
	ErrTransactionActive = errors.New("transaction is active")
)

type TransactionFunc func(ctx context.Context, s storage.Storage) (*CommitSpec, error)

type CommitSpec struct {
	// AuthorName can also be specified when creating the TransactionStorage,
	// but here it can be overridden.
	// +optional
	AuthorName *string
	// AuthorEmail can also be specified when creating the TransactionStorage,
	// but here it can be overridden.
	// +optional
	AuthorEmail *string

	// Message specifies a description of the transaction aim.
	// +required
	Message string
}

type TransactionStorage interface {
	storage.ReadStorage

	Suspend()
	Resume()

	// Pull will force a re-sync from upstream. ErrTransactionActive will be returned
	// if a transaction is active at that time.
	Pull(ctx context.Context) error

	// Transaction creates a new "stream" (for Git: branch) with the given name, or
	// prefix if streamName ends with a dash (in that case, a 8-char hash will be appended).
	// The environment is made sure to be as up-to-date as possible before fn executes. When
	// fn executes, the given storage can be used to modify the desired state. If you want to
	// "commit" the changes made in fn, just return nil. If you want to abort, return ErrAbortTransaction.
	// If you want to 
	Transaction(ctx context.Context, streamName string, fn TransactionFunc) error
}