package lineage

import (
	"context"

	"akyuu/internal/store"
)

// StoreAPI is the interface the lineage workers need from the store.
type StoreAPI interface {
	Query(ctx context.Context, query string, args ...any) (store.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) store.RowScanner
	Exec(ctx context.Context, query string, args ...any) error
	ListSites(ctx context.Context) ([]*store.Site, error)
}
