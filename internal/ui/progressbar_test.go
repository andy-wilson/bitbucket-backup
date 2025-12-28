package ui

import (
	"bytes"
	"testing"
	"time"
)

func TestNewProgressBar(t *testing.T) {
	pb := NewProgressBar(10)
	if pb == nil {
		t.Fatal("NewProgressBar returned nil")
	}
	if pb.total != 10 {
		t.Errorf("expected total=10, got %d", pb.total)
	}
	if pb.width != 40 {
		t.Errorf("expected width=40, got %d", pb.width)
	}
}

func TestProgressBarOptions(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar(10,
		WithBarWriter(&buf),
		WithBarWidth(20),
		WithUpdateInterval(50*time.Millisecond),
	)

	if pb.width != 20 {
		t.Errorf("expected width=20, got %d", pb.width)
	}
	if pb.interval != 50*time.Millisecond {
		t.Errorf("expected interval=50ms, got %v", pb.interval)
	}
}

func TestProgressBarComplete(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar(3, WithBarWriter(&buf))

	pb.SetCurrent("item1")
	pb.Complete("item1")

	c, f := pb.GetStats()
	if c != 1 {
		t.Errorf("expected completed=1, got %d", c)
	}
	if f != 0 {
		t.Errorf("expected failed=0, got %d", f)
	}
}

func TestProgressBarFail(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar(3, WithBarWriter(&buf))

	pb.SetCurrent("item1")
	pb.Fail("item1")

	c, f := pb.GetStats()
	if c != 0 {
		t.Errorf("expected completed=0, got %d", c)
	}
	if f != 1 {
		t.Errorf("expected failed=1, got %d", f)
	}
}

func TestProgressBarStartStop(t *testing.T) {
	var buf bytes.Buffer
	pb := NewProgressBar(5,
		WithBarWriter(&buf),
		WithUpdateInterval(50*time.Millisecond),
	)

	pb.Start()
	time.Sleep(100 * time.Millisecond)

	pb.Complete("item1")
	pb.Complete("item2")
	pb.Fail("item3")

	pb.Stop()

	c, f := pb.GetStats()
	if c != 2 {
		t.Errorf("expected completed=2, got %d", c)
	}
	if f != 1 {
		t.Errorf("expected failed=1, got %d", f)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{3661 * time.Second, "1h01m01s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
