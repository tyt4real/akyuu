package lineage

import (
	"context"
	"fmt"
	"log/slog"
)

type ReplyGraphWorker struct {
	store  StoreAPI
	logger *slog.Logger
}

func NewReplyGraphWorker(st StoreAPI, logger *slog.Logger) *ReplyGraphWorker {
	return &ReplyGraphWorker{store: st, logger: logger}
}

func (w *ReplyGraphWorker) RunOnce(ctx context.Context) error {
	w.logger.Debug("replygraph: refreshing materialized view")

	err := w.store.Exec(ctx, `REFRESH MATERIALIZED VIEW CONCURRENTLY reply_graph_edges`)
	if err != nil {
		w.logger.Warn("replygraph: concurrent refresh failed, trying regular", "err", err)
		err = w.store.Exec(ctx, `REFRESH MATERIALIZED VIEW reply_graph_edges`)
		if err != nil {
			return fmt.Errorf("replygraph: refresh failed: %w", err)
		}
	}

	w.logger.Info("replygraph: refreshed successfully")
	return nil
}
