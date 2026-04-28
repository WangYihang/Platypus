package api

import (
	"context"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
)

// dbTransferRecorder is the production TransferRecorder. It writes
// to the file_transfers table and is safe to share across many
// in-flight transfers — every method just delegates to the
// underlying repo, which uses *sql.DB's connection pool.
type dbTransferRecorder struct {
	db *storage.DB
}

// NewDBTransferRecorder wraps a storage.DB as a TransferRecorder.
func NewDBTransferRecorder(db *storage.DB) TransferRecorder {
	return &dbTransferRecorder{db: db}
}

func (r *dbTransferRecorder) Create(ctx context.Context, ft *storage.FileTransfer) error {
	return r.db.FileTransfers().Create(ctx, ft)
}

func (r *dbTransferRecorder) UpdateProgress(ctx context.Context, id string, bytes, total int64) error {
	return r.db.FileTransfers().UpdateProgress(ctx, id, bytes, total)
}

func (r *dbTransferRecorder) Finish(ctx context.Context, id, status string, bytes int64, errMsg string, at time.Time) error {
	return r.db.FileTransfers().Finish(ctx, id, status, bytes, errMsg, at)
}
