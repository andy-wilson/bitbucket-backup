package ui

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestNewSpinner(t *testing.T) {
	s := NewSpinner()
	if s == nil {
		t.Fatal("NewSpinner returned nil")
	}
	if len(s.frames) == 0 {
		t.Error("frames should not be empty")
	}
	if s.interval != 100*time.Millisecond {
		t.Errorf("interval = %v, want 100ms", s.interval)
	}
}

func TestSpinnerWithOptions(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(
		WithMessage("loading"),
		WithWriter(&buf),
		WithInterval(50*time.Millisecond),
	)

	if s.message != "loading" {
		t.Errorf("message = %q, want %q", s.message, "loading")
	}
	if s.interval != 50*time.Millisecond {
		t.Errorf("interval = %v, want 50ms", s.interval)
	}
}

func TestSpinnerStartStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(
		WithWriter(&buf),
		WithInterval(50*time.Millisecond),
	)

	s.Start()
	time.Sleep(100 * time.Millisecond)
	s.Stop()

	// Should have written something
	if buf.Len() == 0 {
		t.Error("expected some output from spinner")
	}
}

func TestSpinnerDoubleStart(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(
		WithWriter(&buf),
		WithInterval(50*time.Millisecond),
	)

	s.Start()
	s.Start() // Should be a no-op
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}

func TestSpinnerDoubleStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(
		WithWriter(&buf),
		WithInterval(50*time.Millisecond),
	)

	s.Start()
	time.Sleep(50 * time.Millisecond)
	s.Stop()
	s.Stop() // Should be a no-op
}

func TestSpinnerUpdateMessage(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(
		WithWriter(&buf),
		WithMessage("initial"),
		WithInterval(50*time.Millisecond),
	)

	s.Start()
	time.Sleep(60 * time.Millisecond)

	s.UpdateMessage("updated")
	if s.message != "updated" {
		t.Errorf("message = %q, want %q", s.message, "updated")
	}

	time.Sleep(60 * time.Millisecond)
	s.Stop()
}

func TestSpinnerStopWithoutStart(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(WithWriter(&buf))

	// Stopping without starting should be a no-op
	s.Stop()
}

func TestIsTerminal(t *testing.T) {
	var buf bytes.Buffer

	// bytes.Buffer is not a terminal
	if IsTerminal(&buf) {
		t.Error("bytes.Buffer should not be a terminal")
	}

	// os.Stdout might or might not be a terminal depending on environment
	// Just ensure it doesn't panic
	_ = IsTerminal(os.Stdout)
	_ = IsTerminal(os.Stderr)
}

func TestIsTerminal_NonFile(t *testing.T) {
	var buf bytes.Buffer
	if IsTerminal(&buf) {
		t.Error("non-file writer should not be a terminal")
	}
}
