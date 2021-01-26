package transactional

import "context"

// execTransactionsCtx executes the functions in order. Before each
// function in the chain is run; the context is checked for errors
// (e.g. if it has been cancelled or timed out). If a context error
// is returned, or if a function in the chain returns an error, this
// function returns directly, without executing the rest of the
// functions in the chain.
func execTransactionsCtx(ctx context.Context, funcs []txFunc) error {
	for _, fn := range funcs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}
