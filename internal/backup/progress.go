package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Progress tracks and reports backup progress.
type Progress struct {
	mu           sync.Mutex
	startTime    time.Time
	total        int
	completed    int
	failed       int
	current      string
	jsonOutput   bool
	quiet        bool
	lastUpdate   time.Time
	updatePeriod time.Duration
}

// ProgressEvent represents a progress update in JSON format.
type ProgressEvent struct {
	Type       string  `json:"type"`
	Timestamp  string  `json:"timestamp"`
	Total      int     `json:"total"`
	Completed  int     `json:"completed"`
	Failed     int     `json:"failed"`
	Percent    float64 `json:"percent"`
	Current    string  `json:"current,omitempty"`
	Message    string  `json:"message,omitempty"`
	ElapsedSec float64 `json:"elapsed_seconds"`
}

// NewProgress creates a new progress tracker.
func NewProgress(total int, jsonOutput, quiet bool) *Progress {
	return &Progress{
		startTime:    time.Now(),
		total:        total,
		jsonOutput:   jsonOutput,
		quiet:        quiet,
		updatePeriod: 500 * time.Millisecond,
	}
}

// Start marks the start of a new item.
func (p *Progress) Start(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = name
	p.emit("start", fmt.Sprintf("Starting: %s", name))
}

// Complete marks an item as completed.
func (p *Progress) Complete(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.completed++
	p.current = ""
	p.emitProgress("complete", fmt.Sprintf("Completed: %s", name))
}

// Fail marks an item as failed.
func (p *Progress) Fail(name string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.failed++
	p.current = ""
	p.emitProgress("fail", fmt.Sprintf("Failed: %s - %v", name, err))
}

// Update emits a progress update if enough time has passed.
func (p *Progress) Update() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if time.Since(p.lastUpdate) < p.updatePeriod {
		return
	}

	p.emitProgress("progress", "")
}

// Summary prints the final summary.
func (p *Progress) Summary() {
	p.mu.Lock()
	defer p.mu.Unlock()

	elapsed := time.Since(p.startTime)
	msg := fmt.Sprintf("Backup complete: %d/%d succeeded, %d failed in %s",
		p.completed, p.total, p.failed, elapsed.Round(time.Second))

	p.emit("summary", msg)
}

// emitProgress emits a progress event with rate limiting for text output.
func (p *Progress) emitProgress(eventType, message string) {
	if p.quiet && !p.jsonOutput {
		return
	}

	now := time.Now()
	if !p.jsonOutput && time.Since(p.lastUpdate) < p.updatePeriod && eventType == "progress" {
		return
	}
	p.lastUpdate = now

	p.emitLocked(eventType, message)
}

// emit emits a progress event unconditionally.
func (p *Progress) emit(eventType, message string) {
	if p.quiet && !p.jsonOutput {
		return
	}
	p.emitLocked(eventType, message)
}

// emitLocked emits the event (caller must hold lock).
func (p *Progress) emitLocked(eventType, message string) {
	if p.jsonOutput {
		event := ProgressEvent{
			Type:       eventType,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Total:      p.total,
			Completed:  p.completed,
			Failed:     p.failed,
			Percent:    p.percent(),
			Current:    p.current,
			Message:    message,
			ElapsedSec: time.Since(p.startTime).Seconds(),
		}
		data, _ := json.Marshal(event)
		_, _ = fmt.Fprintln(os.Stdout, string(data))
	} else if message != "" {
		fmt.Printf("[%d/%d] %s\n", p.completed+p.failed, p.total, message)
	}
}

// percent calculates completion percentage.
func (p *Progress) percent() float64 {
	if p.total == 0 {
		return 0
	}
	return float64(p.completed+p.failed) / float64(p.total) * 100
}

// GetStats returns the current stats.
func (p *Progress) GetStats() (completed, failed int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.completed, p.failed
}
