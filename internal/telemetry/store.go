package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const retention = 24 * time.Hour

type Event struct {
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	DurationMS   int64     `json:"duration_ms"`
	Success      bool      `json:"success"`
}

type LimitWindow struct {
	UsedPercent      *float64   `json:"used_percent"`
	RemainingPercent *float64   `json:"remaining_percent"`
	ResetAt          *time.Time `json:"reset_at"`
	WindowMinutes    *int       `json:"window_minutes"`
}

type Quota struct {
	PlanType    string      `json:"plan_type,omitempty"`
	Balance     *float64    `json:"balance"`
	HasCredits  *bool       `json:"has_credits"`
	Primary     LimitWindow `json:"primary"`
	Secondary   LimitWindow `json:"secondary"`
	LastUpdated *time.Time  `json:"last_updated"`
}

type Bucket struct {
	Start        time.Time `json:"start"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Requests     int       `json:"requests"`
}

type Totals struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	Requests     int `json:"requests"`
	Errors       int `json:"errors"`
}

type Range struct {
	Hours   int      `json:"hours"`
	Totals  Totals   `json:"totals"`
	Buckets []Bucket `json:"buckets"`
}

type Snapshot struct {
	GeneratedAt    time.Time        `json:"generated_at"`
	StartedAt      time.Time        `json:"started_at"`
	UptimeSeconds  int64            `json:"uptime_seconds"`
	AverageLatency float64          `json:"average_latency_ms"`
	ErrorRate      float64          `json:"error_rate"`
	Ranges         map[string]Range `json:"ranges"`
	Quota          Quota            `json:"quota"`
}

type persisted struct {
	Events []Event `json:"events"`
	Quota  Quota   `json:"quota"`
}

type Store struct {
	mu        sync.RWMutex
	path      string
	startedAt time.Time
	events    []Event
	quota     Quota
}

func New(path string) *Store {
	store := &Store{path: path, startedAt: time.Now()}
	if path == "" {
		return store
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store
	}
	if err != nil {
		slog.Warn("could not read telemetry history", "error", err)
		return store
	}
	var saved persisted
	if err := json.Unmarshal(data, &saved); err != nil {
		slog.Warn("could not decode telemetry history", "error", err)
		return store
	}
	store.events = prune(saved.Events, time.Now())
	store.quota = saved.Quota
	return store
}

func (s *Store) Record(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	s.mu.Lock()
	s.events = append(prune(s.events, event.Timestamp), event)
	s.saveLocked()
	s.mu.Unlock()
}

func (s *Store) UpdateQuota(header http.Header) {
	now := time.Now().UTC()
	s.mu.Lock()
	changed := false
	changed = setString(&s.quota.PlanType, header.Get("X-Codex-Plan-Type")) || changed
	changed = setFloat(&s.quota.Balance, header.Get("X-Codex-Credits-Balance")) || changed
	changed = setBool(&s.quota.HasCredits, header.Get("X-Codex-Credits-Has-Credits")) || changed
	changed = setWindow(&s.quota.Primary, header, "Primary") || changed
	changed = setWindow(&s.quota.Secondary, header, "Secondary") || changed
	if changed {
		s.quota.LastUpdated = &now
		s.saveLocked()
	}
	s.mu.Unlock()
}

func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.RLock()
	events := append([]Event(nil), s.events...)
	quota := s.quota
	startedAt := s.startedAt
	s.mu.RUnlock()

	var latency int64
	errorsCount := 0
	for _, event := range events {
		latency += event.DurationMS
		if !event.Success {
			errorsCount++
		}
	}
	averageLatency, errorRate := 0.0, 0.0
	if len(events) > 0 {
		averageLatency = float64(latency) / float64(len(events))
		errorRate = float64(errorsCount) / float64(len(events)) * 100
	}
	return Snapshot{
		GeneratedAt: now, StartedAt: startedAt, UptimeSeconds: int64(now.Sub(startedAt).Seconds()),
		AverageLatency: averageLatency, ErrorRate: errorRate, Quota: quota,
		Ranges: map[string]Range{
			"1h":  buildRange(events, now, time.Hour, 5*time.Minute),
			"5h":  buildRange(events, now, 5*time.Hour, 15*time.Minute),
			"12h": buildRange(events, now, 12*time.Hour, 30*time.Minute),
			"24h": buildRange(events, now, 24*time.Hour, time.Hour),
		},
	}
}

func buildRange(events []Event, now time.Time, duration, resolution time.Duration) Range {
	end := now.Truncate(resolution).Add(resolution)
	start := end.Add(-duration)
	count := int(duration / resolution)
	buckets := make([]Bucket, count)
	for index := range buckets {
		buckets[index].Start = start.Add(time.Duration(index) * resolution)
	}
	totals := Totals{}
	for _, event := range events {
		if event.Timestamp.Before(start) || !event.Timestamp.Before(end) {
			continue
		}
		index := int(event.Timestamp.Sub(start) / resolution)
		if index < 0 || index >= len(buckets) {
			continue
		}
		bucket := &buckets[index]
		bucket.InputTokens += event.InputTokens
		bucket.OutputTokens += event.OutputTokens
		bucket.Requests++
		totals.InputTokens += event.InputTokens
		totals.OutputTokens += event.OutputTokens
		totals.Requests++
		if !event.Success {
			totals.Errors++
		}
	}
	return Range{Hours: int(duration / time.Hour), Totals: totals, Buckets: buckets}
}

func prune(events []Event, now time.Time) []Event {
	cutoff := now.Add(-retention)
	first := 0
	for first < len(events) && events[first].Timestamp.Before(cutoff) {
		first++
	}
	return append([]Event(nil), events[first:]...)
}

func (s *Store) saveLocked() {
	if s.path == "" {
		return
	}
	data, err := json.Marshal(persisted{Events: s.events, Quota: s.quota})
	if err != nil {
		slog.Warn("could not encode telemetry history", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		slog.Warn("could not create telemetry directory", "error", err)
		return
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".metrics-*.json")
	if err != nil {
		slog.Warn("could not create telemetry file", "error", err)
		return
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	_ = temporary.Chmod(0o600)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		slog.Warn("could not write telemetry history", "error", err)
		return
	}
	if err := temporary.Close(); err != nil {
		slog.Warn("could not close telemetry history", "error", err)
		return
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		_ = os.Remove(s.path)
		if err := os.Rename(temporaryPath, s.path); err != nil {
			slog.Warn("could not replace telemetry history", "error", err)
		}
	}
}

func setWindow(window *LimitWindow, header http.Header, name string) bool {
	changed := false
	changed = setFloat(&window.UsedPercent, header.Get("X-Codex-"+name+"-Used-Percent")) || changed
	if window.UsedPercent != nil {
		remaining := 100 - *window.UsedPercent
		if remaining < 0 {
			remaining = 0
		}
		window.RemainingPercent = &remaining
	}
	changed = setTime(&window.ResetAt, header.Get("X-Codex-"+name+"-Reset-At")) || changed
	changed = setInt(&window.WindowMinutes, header.Get("X-Codex-"+name+"-Window-Minutes")) || changed
	return changed
}

func setString(target *string, value string) bool {
	if value == "" || *target == value {
		return false
	}
	*target = value
	return true
}

func setFloat(target **float64, value string) bool {
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false
	}
	if *target != nil && **target == parsed {
		return false
	}
	*target = &parsed
	return true
}

func setInt(target **int, value string) bool {
	if value == "" {
		return false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return false
	}
	if *target != nil && **target == parsed {
		return false
	}
	*target = &parsed
	return true
}

func setBool(target **bool, value string) bool {
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	if *target != nil && **target == parsed {
		return false
	}
	*target = &parsed
	return true
}

func setTime(target **time.Time, value string) bool {
	if value == "" {
		return false
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return false
	}
	parsed := time.Unix(seconds, 0).UTC()
	if *target != nil && (*target).Equal(parsed) {
		return false
	}
	*target = &parsed
	return true
}

func (s Snapshot) String() string {
	return fmt.Sprintf("requests=%d uptime=%s", s.Ranges["24h"].Totals.Requests, time.Duration(s.UptimeSeconds)*time.Second)
}
