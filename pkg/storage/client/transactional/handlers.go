package transactional

import "context"

type TxInfo struct {
	Base    string
	Head    string
	Options TxOptions
}

type CommitHandler interface {
	HandlePreCommit(ctx context.Context, commit Commit, info TxInfo) error
	HandlePostCommit(ctx context.Context, commit Commit, info TxInfo) error
}

type MultiCommitHandler struct {
	CommitHandlers []CommitHandler
}

func (m *MultiCommitHandler) HandlePreCommit(ctx context.Context, commit Commit, info TxInfo) error {
	for _, ch := range m.CommitHandlers {
		if ch == nil {
			continue
		}
		if err := ch.HandlePreCommit(ctx, commit, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiCommitHandler) HandlePostCommit(ctx context.Context, commit Commit, info TxInfo) error {
	for _, ch := range m.CommitHandlers {
		if ch == nil {
			continue
		}
		if err := ch.HandlePostCommit(ctx, commit, info); err != nil {
			return err
		}
	}
	return nil
}

type TransactionHandler interface {
	HandlePreTransaction(ctx context.Context, info TxInfo) error
	HandlePostTransaction(ctx context.Context, info TxInfo) error
}

type MultiTransactionHandler struct {
	TransactionHandlers []TransactionHandler
}

func (m *MultiTransactionHandler) HandlePreTransaction(ctx context.Context, info TxInfo) error {
	for _, th := range m.TransactionHandlers {
		if th == nil {
			continue
		}
		if err := th.HandlePreTransaction(ctx, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiTransactionHandler) HandlePostTransaction(ctx context.Context, info TxInfo) error {
	for _, th := range m.TransactionHandlers {
		if th == nil {
			continue
		}
		if err := th.HandlePostTransaction(ctx, info); err != nil {
			return err
		}
	}
	return nil
}
