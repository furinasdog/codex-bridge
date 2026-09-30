package telemetry

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreBuildsRangesAndPersists(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "metrics.json")
	store := New(path)
	store.Record(Event{Timestamp: now.Add(-30 * time.Minute), Model: "gpt-test", InputTokens: 120, OutputTokens: 30, DurationMS: 200, Success: true})
	store.Record(Event{Timestamp: now.Add(-2 * time.Hour), Model: "gpt-test", InputTokens: 50, OutputTokens: 10, DurationMS: 400, Success: false})

	snapshot := store.Snapshot(now)
	if got := snapshot.Ranges["1h"].Totals.InputTokens; got != 120 {
		t.Fatalf("1h input tokens = %d, want 120", got)
	}
	if got := snapshot.Ranges["5h"].Totals.Requests; got != 2 {
		t.Fatalf("5h requests = %d, want 2", got)
	}
	if snapshot.ErrorRate != 50 || snapshot.AverageLatency != 300 {
		t.Fatalf("unexpected service metrics: error rate %.1f, latency %.1f", snapshot.ErrorRate, snapshot.AverageLatency)
	}

	reloaded := New(path).Snapshot(now)
	if got := reloaded.Ranges["5h"].Totals.OutputTokens; got != 40 {
		t.Fatalf("persisted output tokens = %d, want 40", got)
	}
}

func TestStoreReadsCodexQuotaHeaders(t *testing.T) {
	t.Parallel()
	store := New("")
	header := make(http.Header)
	header.Set("X-Codex-Plan-Type", "pro")
	header.Set("X-Codex-Credits-Balance", "18.5")
	header.Set("X-Codex-Credits-Has-Credits", "true")
	header.Set("X-Codex-Primary-Used-Percent", "37.5")
	header.Set("X-Codex-Primary-Reset-At", "1790755200")
	header.Set("X-Codex-Primary-Window-Minutes", "300")
	store.UpdateQuota(header)

	quota := store.Snapshot(time.Now()).Quota
	if quota.PlanType != "pro" || quota.Balance == nil || *quota.Balance != 18.5 {
		t.Fatalf("unexpected quota: %#v", quota)
	}
	if quota.Primary.RemainingPercent == nil || *quota.Primary.RemainingPercent != 62.5 {
		t.Fatalf("remaining percent = %#v, want 62.5", quota.Primary.RemainingPercent)
	}
	if quota.Primary.WindowMinutes == nil || *quota.Primary.WindowMinutes != 300 || quota.Primary.ResetAt == nil {
		t.Fatalf("unexpected primary window: %#v", quota.Primary)
	}
}
