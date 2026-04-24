package activity

import (
	"context"
	"log/slog"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
)

// RetentionProvider supplies the current retention window in days.
// Implementations consult the settings registry live so a runtime
// edit is picked up on the next reap tick. 0 disables retention.
type RetentionProvider interface {
	AuditRetentionDays() int
}

// Reaper runs a background loop that deletes audit rows older than
// the current retention window. It re-reads the window on every tick
// so admin edits take effect without a restart.
type Reaper struct {
	db       *storage.DB
	settings RetentionProvider
	logger   *slog.Logger
	// interval between sweeps. Default 1 hour; kept short enough
	// that edits feel responsive without pounding the DB.
	interval time.Duration
}

// NewReaper constructs a Reaper. db must be non-nil. settings may
// return 0 from AuditRetentionDays to disable retention; the reaper
// then runs but issues no deletes.
func NewReaper(db *storage.DB, settings RetentionProvider, logger *slog.Logger) *Reaper {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reaper{
		db:       db,
		settings: settings,
		logger:   logger.With(slog.String("component", "activity.reaper")),
		interval: time.Hour,
	}
}

// Run blocks until ctx is cancelled, sweeping on the configured
// interval. Errors from the delete are logged but never propagated
// — the audit log is best-effort maintenance.
func (r *Reaper) Run(ctx context.Context) {
	// First sweep after one interval, not immediately, to let the
	// server finish booting before touching the DB.
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sweep(ctx)
		}
	}
}

func (r *Reaper) sweep(ctx context.Context) {
	days := r.settings.AuditRetentionDays()
	if days <= 0 {
		return // retention disabled
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	n, err := r.db.Activities().DeleteOlderThan(ctx, cutoff)
	if err != nil {
		r.logger.Warn("delete older than",
			slog.String("cutoff", cutoff.Format(time.RFC3339)),
			slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		r.logger.Info("purged",
			slog.Int64("rows", n),
			slog.Int("retention_days", days),
			slog.String("cutoff", cutoff.Format(time.RFC3339)))
	}
}
