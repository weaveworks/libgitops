package transaction

import (
	"context"
	"errors"

	"github.com/weaveworks/libgitops/pkg/storage"
)

var (
	ErrAbortTransaction      = errors.New("transaction aborted")
	ErrTransactionActive     = errors.New("transaction is active")
	ErrNoPullRequestProvider = errors.New("no pull request provider given")
)

type TransactionFunc func(ctx context.Context, s storage.Storage) (CommitResult, error)

type TransactionStorage interface {
	storage.ReadStorage

	// Transaction creates a new "stream" (for Git: branch) with the given name, or
	// prefix if streamName ends with a dash (in that case, a 8-char hash will be appended).
	// The environment is made sure to be as up-to-date as possible before fn executes. When
	// fn executes, the given storage can be used to modify the desired state. If you want to
	// "commit" the changes made in fn, just return nil. If you want to abort, return ErrAbortTransaction.
	// If you want to
	Transaction(ctx context.Context, streamName string, fn TransactionFunc) error
}
