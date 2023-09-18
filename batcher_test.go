package smallbatch

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mocks "github.com/yjimk/smallbatch/mocks"
	"github.com/yjimk/smallbatch/types"
)

func TestNewBatcher(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProcessor := mocks.NewMockBatchProcessor(ctrl)
	sampleConfig := Config{
		BatchSize:    2,
		TimeInterval: 100,
	}

	batcher := NewBatcher(mockProcessor, sampleConfig)

	// Verify config
	assert.Equal(t, sampleConfig, batcher.config)

	// Verify batchProcessor
	assert.Equal(t, mockProcessor, batcher.batchProcessor)

	// Check if channels and maps are initialised
	assert.NotNil(t, batcher.jobs)
	assert.NotNil(t, batcher.results)
	assert.NotNil(t, batcher.shutdown)

	assert.Equal(t, sampleConfig.BatchSize, cap(batcher.jobs))

	// Check the batcher isn't closed
	assert.False(t, batcher.closed.Load().(bool))

	// Check if resultChanPool is initialised
	assert.NotNil(t, batcher.resultChanPool.New())
	assert.IsType(t, make(chan types.JobResult, 1), batcher.resultChanPool.New())
}

func TestBatcher_SubmitJob(t *testing.T) {
	sampleConfig := Config{
		BatchSize:    2,
		TimeInterval: 100 * time.Millisecond,
	}

	tests := []struct {
		name           string
		jobs           []types.Job
		setup          func(b *Batcher)
		mockReturns    []types.JobResult
		want           []types.JobResult
		afterTestCheck func(b *Batcher)
	}{
		{
			name:        "valid job submission",
			jobs:        []types.Job{{ID: "1", Payload: "sample"}},
			mockReturns: []types.JobResult{{JobID: "1", Err: nil}},
			want:        []types.JobResult{{JobID: "1", BatchNumber: 0, Err: nil}},
		},
		{
			name:        "processor returns error",
			jobs:        []types.Job{{ID: "1", Payload: "sample"}},
			mockReturns: []types.JobResult{{JobID: "1", Err: errors.New("an error occurred")}},
			want:        []types.JobResult{{JobID: "1", BatchNumber: 0, Err: errors.New("an error occurred")}},
		},
		{
			name:  "submit job after batcher is shut down",
			jobs:  []types.Job{{ID: "2", Payload: "sample2"}},
			setup: func(b *Batcher) { b.Shutdown() },
			want:  []types.JobResult{{Err: errors.New("batcher is closed")}},
		},
		{
			name:        "ensure result channel pool cleared",
			jobs:        []types.Job{{ID: "3", Payload: "sample4"}},
			mockReturns: []types.JobResult{{Err: nil}},
			want:        []types.JobResult{{JobID: "3", BatchNumber: 0, Err: nil}},
			afterTestCheck: func(b *Batcher) {
				b.mu.Lock()
				assert.Zero(t, len(b.results), "Result channel pool leak: got %v, want 0", len(b.results))
				b.mu.Unlock()
			},
		},
		{
			name:        "multiple concurrent job submissions",
			jobs:        []types.Job{{ID: "4", Payload: "sample5"}, {ID: "5", Payload: "sample6"}},
			mockReturns: []types.JobResult{{JobID: "4", Err: nil}, {JobID: "5", Err: nil}},
			want:        []types.JobResult{{JobID: "4", BatchNumber: 0, Err: nil}, {JobID: "5", BatchNumber: 0, Err: nil}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wg sync.WaitGroup

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockProcessor := mocks.NewMockBatchProcessor(ctrl)
			for _, mockReturn := range tt.mockReturns {
				mockProcessor.EXPECT().Process(gomock.Any()).Return(mockReturn).AnyTimes()
			}

			batcher := NewBatcher(mockProcessor, sampleConfig)
			if tt.setup != nil {
				tt.setup(batcher)
			}
			go batcher.Start()

			var results []types.JobResult
			for _, job := range tt.jobs {
				wg.Add(1)
				go func(j types.Job) {
					defer wg.Done()
					results = append(results, batcher.SubmitJob(j))
				}(job)
			}
			wg.Wait()

			for _, got := range results {
				assert.Contains(t, tt.want, got, "Expected job result is not present")
			}

			if tt.afterTestCheck != nil {
				tt.afterTestCheck(batcher)
			}
		})
	}
}

func TestBatcher_Start(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sampleConfig := Config{
		BatchSize:    3, // Increased for ticker test case
		TimeInterval: 100 * time.Millisecond,
	}

	t.Run("recieves single job then shuts down", func(t *testing.T) {
		mockProcessor := mocks.NewMockBatchProcessor(ctrl)
		mockProcessor.EXPECT().Process(gomock.Any()).Return(types.JobResult{Err: nil}).Times(1)

		batcher := NewBatcher(mockProcessor, sampleConfig)

		go batcher.Start()

		batcher.SubmitJob(types.Job{ID: "1"})
		batcher.Shutdown()
	})

	t.Run("should process multiple sequential job submissions", func(t *testing.T) {
		jobs := 2
		mockProcessor := mocks.NewMockBatchProcessor(ctrl)
		mockProcessor.EXPECT().Process(gomock.Any()).Return(types.JobResult{Err: nil}).Times(jobs)

		batcher := NewBatcher(mockProcessor, sampleConfig)

		go batcher.Start()

		for i := 0; i < jobs; i++ {
			batcher.SubmitJob(types.Job{ID: fmt.Sprintf("%d", i)})
		}

		batcher.Shutdown()
	})

	t.Run("should process multiple concurrent job submissions", func(t *testing.T) {
		var wg sync.WaitGroup

		jobs := 10

		mockProcessor := mocks.NewMockBatchProcessor(ctrl)
		mockProcessor.EXPECT().Process(gomock.Any()).Return(types.JobResult{Err: nil}).Times(jobs)

		batcher := NewBatcher(mockProcessor, sampleConfig)

		go batcher.Start()

		for i := 0; i < jobs; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				batcher.SubmitJob(types.Job{ID: fmt.Sprintf("%d", index)})
			}(i)
		}

		wg.Wait()

		batcher.Shutdown()
	})
}

func TestBatcher_Shutdown(t *testing.T) {
	// Initialise the Batcher.
	batcher := &Batcher{
		jobs:         make(chan types.Job),
		shutdown:     make(chan struct{}, 1),
		shutdownOnce: &sync.Once{},
		closed:       atomic.Value{},
		wg:           sync.WaitGroup{},
	}
	batcher.closed.Store(false) // Set initial state to not closed.

	// Pretend a jobs is being processed.
	batcher.wg.Add(1)

	// Initiate shutdown.
	done := batcher.Shutdown()

	// Check that jobs channel is closed.
	_, open := <-batcher.jobs
	assert.False(t, open, "Expected jobs channel to be closed after Shutdown")

	// Finish the remaining job.
	batcher.wg.Done()

	// Check if shutdown is complete.
	_, open = <-done
	assert.False(t, open, "Expected done channel to be closed after all jobs are processed")

	// Check if shutdown channel is closed.
	_, open = <-batcher.shutdown
	assert.False(t, open, "Expected shutdown channel to be closed after Shutdown")

	// Check if batcher is marked as closed.
	closed := batcher.closed.Load().(bool)
	assert.True(t, closed, "Expected batcher to be marked as closed after Shutdown")
}
