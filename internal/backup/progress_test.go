package backup

import (
	"testing"
)

func TestNewProgress(t *testing.T) {
	p := NewProgress(10, false, false, false)
	if p == nil {
		t.Fatal("NewProgress returned nil")
	}
	if p.total != 10 {
		t.Errorf("total = %d, want 10", p.total)
	}
	if p.jsonOutput {
		t.Error("jsonOutput should be false")
	}
	if p.quiet {
		t.Error("quiet should be false")
	}
	if p.interactive {
		t.Error("interactive should be false")
	}
}

func TestNewProgress_JSON(t *testing.T) {
	p := NewProgress(5, true, false, false)
	if !p.jsonOutput {
		t.Error("jsonOutput should be true")
	}
}

func TestNewProgress_Quiet(t *testing.T) {
	p := NewProgress(5, false, true, false)
	if !p.quiet {
		t.Error("quiet should be true")
	}
}

func TestProgress_StartComplete(t *testing.T) {
	p := NewProgress(2, false, true, false) // quiet mode to avoid output

	p.Start("repo1")
	if p.active.Load() != 1 {
		t.Errorf("active = %d, want 1", p.active.Load())
	}

	p.Complete("repo1")
	if p.active.Load() != 0 {
		t.Errorf("active = %d, want 0", p.active.Load())
	}
	if p.completed.Load() != 1 {
		t.Errorf("completed = %d, want 1", p.completed.Load())
	}
}

func TestProgress_StartWithType(t *testing.T) {
	p := NewProgress(2, false, true, false) // quiet mode to avoid output

	p.StartWithType("repo1", "cloning")
	if p.current != "cloning: repo1" {
		t.Errorf("current = %q, want %q", p.current, "cloning: repo1")
	}
	p.Complete("repo1")
}

func TestProgress_Fail(t *testing.T) {
	p := NewProgress(2, false, true, false) // quiet mode

	p.Start("repo1")
	p.Fail("repo1", nil)

	if p.failed.Load() != 1 {
		t.Errorf("failed = %d, want 1", p.failed.Load())
	}
	if p.active.Load() != 0 {
		t.Errorf("active = %d, want 0", p.active.Load())
	}
}

func TestProgress_Interrupt(t *testing.T) {
	p := NewProgress(2, false, true, false) // quiet mode

	p.Start("repo1")
	p.Interrupt("repo1")

	if p.interrupted.Load() != 1 {
		t.Errorf("interrupted = %d, want 1", p.interrupted.Load())
	}
	if p.active.Load() != 0 {
		t.Errorf("active = %d, want 0", p.active.Load())
	}
}

func TestProgress_GetStats(t *testing.T) {
	p := NewProgress(5, false, true, false) // quiet mode

	p.Start("repo1")
	p.Complete("repo1")

	p.Start("repo2")
	p.Fail("repo2", nil)

	completed, failed := p.GetStats()
	if completed != 1 {
		t.Errorf("completed = %d, want 1", completed)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
}

func TestProgress_percent(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		completed int
		failed    int
		want      float64
	}{
		{"empty", 0, 0, 0, 0},
		{"none done", 10, 0, 0, 0},
		{"half done", 10, 5, 0, 50},
		{"all done", 10, 10, 0, 100},
		{"with failures", 10, 5, 5, 100},
		{"partial with failures", 10, 3, 2, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProgress(tt.total, false, true, false)
			for i := 0; i < tt.completed; i++ {
				p.Start("repo")
				p.Complete("repo")
			}
			for i := 0; i < tt.failed; i++ {
				p.Start("repo")
				p.Fail("repo", nil)
			}

			got := p.percent()
			if got != tt.want {
				t.Errorf("percent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProgress_UpdateStatus(t *testing.T) {
	p := NewProgress(10, false, true, false) // quiet mode

	p.UpdateStatus("fetching PRs: repo1")
	if p.current != "fetching PRs: repo1" {
		t.Errorf("current = %q, want %q", p.current, "fetching PRs: repo1")
	}

	p.UpdateStatus("saving PRs: repo1 (5/10)")
	if p.current != "saving PRs: repo1 (5/10)" {
		t.Errorf("current = %q, want %q", p.current, "saving PRs: repo1 (5/10)")
	}
}

func TestProgress_ConcurrentStartComplete(t *testing.T) {
	p := NewProgress(100, false, true, false) // quiet mode

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				p.Start("repo")
				p.Complete("repo")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	completed, failed := p.GetStats()
	if completed != 100 {
		t.Errorf("completed = %d, want 100", completed)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
}

func TestProgress_Update(t *testing.T) {
	p := NewProgress(10, false, true, false) // quiet mode

	// Update should not panic
	p.Update()
}

func TestProgress_Summary(t *testing.T) {
	p := NewProgress(2, false, true, false) // quiet mode

	p.Start("repo1")
	p.Complete("repo1")
	p.Start("repo2")
	p.Fail("repo2", nil)

	// Summary should not panic
	p.Summary()
}
