package transactional

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// Implement the required "fluent/functional" methods on BranchTx.
// Go doesn't have generics; hence we need to do this twice.

func (tx *txImpl) Get(key core.ObjectKey, obj core.Object) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Get(ctx, key, obj)
	})
}
func (tx *txImpl) List(list core.ObjectList, opts ...core.ListOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.List(ctx, list, opts...)
	})
}

func (tx *txImpl) Create(obj core.Object, opts ...core.CreateOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Create(ctx, obj, opts...)
	})
}
func (tx *txImpl) Update(obj core.Object, opts ...core.UpdateOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Update(ctx, obj, opts...)
	})
}
func (tx *txImpl) Patch(obj core.Object, patch core.Patch, opts ...core.PatchOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Patch(ctx, obj, patch, opts...)
	})
}
func (tx *txImpl) Delete(obj core.Object, opts ...core.DeleteOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Delete(ctx, obj, opts...)
	})
}
func (tx *txImpl) DeleteAllOf(obj core.Object, opts ...core.DeleteAllOfOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.DeleteAllOf(ctx, obj, opts...)
	})
}

func (tx *txImpl) UpdateStatus(obj core.Object, opts ...core.UpdateOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Update(ctx, obj, opts...)
	})
}
func (tx *txImpl) PatchStatus(obj core.Object, patch core.Patch, opts ...core.PatchOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Patch(ctx, obj, patch, opts...)
	})
}

// Implement the required "fluent/functional" methods on BranchTx.
// Go doesn't have generics; hence we need to do this twice.

func (tx *txBranchImpl) Get(key core.ObjectKey, obj core.Object) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Get(ctx, key, obj)
	})
}
func (tx *txBranchImpl) List(list core.ObjectList, opts ...core.ListOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.List(ctx, list, opts...)
	})
}

func (tx *txBranchImpl) Create(obj core.Object, opts ...core.CreateOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Create(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) Update(obj core.Object, opts ...core.UpdateOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Update(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) Patch(obj core.Object, patch core.Patch, opts ...core.PatchOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Patch(ctx, obj, patch, opts...)
	})
}
func (tx *txBranchImpl) Delete(obj core.Object, opts ...core.DeleteOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Delete(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) DeleteAllOf(obj core.Object, opts ...core.DeleteAllOfOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.DeleteAllOf(ctx, obj, opts...)
	})
}

func (tx *txBranchImpl) UpdateStatus(obj core.Object, opts ...core.UpdateOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Update(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) PatchStatus(obj core.Object, patch core.Patch, opts ...core.PatchOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Patch(ctx, obj, patch, opts...)
	})
}
