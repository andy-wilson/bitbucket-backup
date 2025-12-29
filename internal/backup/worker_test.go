package backup

import (
	"context"
	"errors"
	"testing"
)

func TestGenerateJobID(t *testing.T) {
	id1 := generateJobID()
	id2 := generateJobID()

	if id1 == "" {
		t.Error("generateJobID() returned empty string")
	}
	if len(id1) != 8 {
		t.Errorf("generateJobID() length = %d, want 8", len(id1))
	}
	if id1 == id2 {
		t.Error("generateJobID() should return unique IDs")
	}
}

func TestNewWorkerPool(t *testing.T) {
	logFunc := func(msg string, args ...interface{}) {}

	tests := []struct {
		name       string
		workers    int
		totalJobs  int
		maxRetry   int
		wantBuffer int
	}{
		{"small pool", 2, 5, 2, 15},           // 5 + 5*2 = 15
		{"larger pool", 4, 10, 3, 40},         // 10 + 10*3 = 40
		{"min buffer", 4, 1, 0, 8},            // min is workers*2
		{"zero jobs", 2, 0, 0, 4},             // min is workers*2
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := newWorkerPool(tt.workers, tt.totalJobs, tt.maxRetry, logFunc)

			if pool == nil {
				t.Fatal("newWorkerPool returned nil")
			}
			if pool.workers != tt.workers {
				t.Errorf("workers = %d, want %d", pool.workers, tt.workers)
			}
			if pool.jobBuffer < tt.wantBuffer {
				t.Errorf("jobBuffer = %d, want >= %d", pool.jobBuffer, tt.wantBuffer)
			}
			if pool.logFunc == nil {
				t.Error("logFunc should not be nil")
			}
		})
	}
}

func TestWorkerPool_ShouldRetry(t *testing.T) {
	pool := newWorkerPool(2, 5, 3, nil)

	tests := []struct {
		name    string
		job     repoJob
		err     error
		want    bool
	}{
		{
			name:    "first attempt",
			job:     repoJob{attempt: 0, maxRetry: 3},
			err:     errors.New("some error"),
			want:    true,
		},
		{
			name:    "max retries reached",
			job:     repoJob{attempt: 3, maxRetry: 3},
			err:     errors.New("some error"),
			want:    false,
		},
		{
			name:    "context canceled",
			job:     repoJob{attempt: 0, maxRetry: 3},
			err:     context.Canceled,
			want:    false,
		},
		{
			name:    "deadline exceeded",
			job:     repoJob{attempt: 0, maxRetry: 3},
			err:     context.DeadlineExceeded,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pool.shouldRetry(tt.job, tt.err)
			if got != tt.want {
				t.Errorf("shouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsGoGitRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"regular error", errors.New("connection refused"), false},
		{"packfile nil", errors.New("packfile is nil"), true},
		{"nil pointer", errors.New("nil pointer dereference"), true},
		{"unexpected EOF", errors.New("unexpected EOF"), true},
		{"go-git panic", errors.New("go-git panic recovered"), true},
		{"reference delta", errors.New("reference delta not found"), true},
		{"invalid memory", errors.New("invalid memory address or nil pointer dereference"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGoGitRetryableError(tt.err)
			if got != tt.want {
				t.Errorf("isGoGitRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkerPool_Stats(t *testing.T) {
	pool := newWorkerPool(2, 5, 3, nil)

	stats := pool.stats()
	if stats == "" {
		t.Error("stats() returned empty string")
	}
}

func TestWorkerPool_Submit(t *testing.T) {
	pool := newWorkerPool(2, 5, 3, nil)

	job := repoJob{
		baseDir:  "/tmp",
		repo:     nil,
		attempt:  0,
		maxRetry: 3,
		jobID:    "test-job",
	}

	pool.submit(job)

	if pool.jobsSubmitted.Load() != 1 {
		t.Errorf("jobsSubmitted = %d, want 1", pool.jobsSubmitted.Load())
	}
}

func TestWorkerPool_Close(t *testing.T) {
	pool := newWorkerPool(2, 5, 3, nil)

	// Close once
	pool.close()

	// Jobs channel should be closed
	_, ok := <-pool.jobs
	if ok {
		t.Error("jobs channel should be closed")
	}
}

func TestWorkerPool_MarkResultRead(t *testing.T) {
	pool := newWorkerPool(2, 5, 3, nil)

	pool.markResultRead()
	if pool.resultsRead.Load() != 1 {
		t.Errorf("resultsRead = %d, want 1", pool.resultsRead.Load())
	}

	pool.markResultRead()
	if pool.resultsRead.Load() != 2 {
		t.Errorf("resultsRead = %d, want 2", pool.resultsRead.Load())
	}
}
