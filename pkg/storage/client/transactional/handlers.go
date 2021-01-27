package transactional

import "context"

type TxInfo struct {
	Base    string
	Head    string
	Options TxOptions
}

type CommitHookChain interface {
	// The chain also itself implements CommitHook
	CommitHook
	// Register registers a new CommitHook to the chain
	Register(CommitHook)
}

type CommitHook interface {
	PreCommitHook(ctx context.Context, commit Commit, info TxInfo) error
	PostCommitHook(ctx context.Context, commit Commit, info TxInfo) error
}

var _ CommitHookChain = &MultiCommitHook{}
var _ CommitHook = &MultiCommitHook{}

type MultiCommitHook struct {
	CommitHooks []CommitHook
}

func (m *MultiCommitHook) Register(h CommitHook) {
	m.CommitHooks = append(m.CommitHooks, h)
}

func (m *MultiCommitHook) PreCommitHook(ctx context.Context, commit Commit, info TxInfo) error {
	for _, ch := range m.CommitHooks {
		if ch == nil {
			continue
		}
		if err := ch.PreCommitHook(ctx, commit, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiCommitHook) PostCommitHook(ctx context.Context, commit Commit, info TxInfo) error {
	for _, ch := range m.CommitHooks {
		if ch == nil {
			continue
		}
		if err := ch.PostCommitHook(ctx, commit, info); err != nil {
			return err
		}
	}
	return nil
}

type TransactionHookChain interface {
	// The chain also itself implements TransactionHook
	TransactionHook
	// Register registers a new CommitHook to the chain
	Register(TransactionHook)
}

type TransactionHook interface {
	PreTransactionHook(ctx context.Context, info TxInfo) error
	PostTransactionHook(ctx context.Context, info TxInfo) error
}

var _ TransactionHookChain = &MultiTransactionHook{}
var _ TransactionHook = &MultiTransactionHook{}

type MultiTransactionHook struct {
	TransactionHooks []TransactionHook
}

func (m *MultiTransactionHook) Register(h TransactionHook) {
	m.TransactionHooks = append(m.TransactionHooks, h)
}

func (m *MultiTransactionHook) PreTransactionHook(ctx context.Context, info TxInfo) error {
	for _, th := range m.TransactionHooks {
		if th == nil {
			continue
		}
		if err := th.PreTransactionHook(ctx, info); err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiTransactionHook) PostTransactionHook(ctx context.Context, info TxInfo) error {
	for _, th := range m.TransactionHooks {
		if th == nil {
			continue
		}
		if err := th.PostTransactionHook(ctx, info); err != nil {
			return err
		}
	}
	return nil
}
