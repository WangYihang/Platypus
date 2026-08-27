package recording

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/WangYihang/Platypus/internal/llm"
	"github.com/WangYihang/Platypus/internal/log"
)

// summariseTimeout bounds one LLM round-trip end to end, including
// reading the cast file. Generous relative to a chat completion but
// far short of the shutdown grace window, so Wait never becomes the
// long pole when the process is going down.
const summariseTimeout = 60 * time.Second

// maxConcurrentSummaries caps how many LLM calls are in flight at
// once. A fleet-wide disconnect finishes many sessions at the same
// instant, and without a cap that turns into a burst of outbound
// requests that the provider will rate-limit anyway.
const maxConcurrentSummaries = 4

// summarise kicks off the LLM summary for a finished recording and
// returns immediately. Never blocks Session.Finish: the caller is on
// the terminal-close path and the round-trip is a network call.
//
// Skipped silently when no LLM is configured, which is the default —
// see llm.FromEnv.
func (m *Manager) summarise(recID string) {
	if m.llm == nil || !m.llm.Available() {
		return
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		// Deliberately not derived from Finish's context. That one is
		// request-scoped and is cancelled the moment the terminal
		// handler returns, which is almost always before the LLM
		// answers.
		ctx, cancel := context.WithTimeout(context.Background(), summariseTimeout)
		defer cancel()

		select {
		case m.sem <- struct{}{}:
			defer func() { <-m.sem }()
		case <-ctx.Done():
			log.Warn("recording: summarise %s gave up waiting for a slot", recID)
			return
		}

		if err := m.summariseOne(ctx, recID); err != nil {
			// A failed summary is cosmetic: the recording itself is
			// already persisted and plays back fine. Log and move on
			// rather than retrying into a provider that may be down.
			log.Warn("recording: summarise %s: %v", recID, err)
		}
	}()
}

// summariseOne does the actual work: re-read the row, check the
// project opted in, extract + redact the terminal text, ask the model,
// store the answer.
func (m *Manager) summariseOne(ctx context.Context, recID string) error {
	rec, err := m.db.TerminalRecordings().Get(ctx, recID)
	if err != nil {
		return fmt.Errorf("load recording: %w", err)
	}

	// The opt-in is re-read here rather than passed down from Finish
	// so that turning the setting off takes effect for sessions that
	// are already closing.
	proj, err := m.db.Projects().GetByID(ctx, rec.ProjectID)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if !proj.AISummariesEnabled {
		return nil
	}

	path := m.PathFor(rec)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open cast: %w", err)
	}
	defer func() { _ = f.Close() }()

	text, err := llm.ExtractFromCast(f)
	if err != nil {
		return fmt.Errorf("extract cast: %w", err)
	}
	if text == "" {
		return nil
	}

	// Redact before the text leaves the deployment. Best-effort by
	// construction (see llm.Redact) — the count is logged so an
	// operator reviewing a leak has a signal that scrubbing ran.
	clean, redacted := llm.Redact(text)
	if redacted > 0 {
		log.Info("recording: summarise %s redacted %d secret-shaped span(s)",
			recID, redacted)
	}

	summary, err := m.llm.Summarise(ctx, clean)
	if err != nil {
		return fmt.Errorf("summarise: %w", err)
	}
	if summary == "" {
		return nil
	}
	if err := m.db.TerminalRecordings().SetSummary(ctx, recID, summary); err != nil {
		return fmt.Errorf("store summary: %w", err)
	}
	return nil
}

// Wait blocks until every in-flight summariser has finished. Called
// from the server's shutdown path so the process does not exit with
// LLM calls still running and their DB writes half-done.
func (m *Manager) Wait() {
	if m == nil {
		return
	}
	m.wg.Wait()
}
