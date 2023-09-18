package smallbatch

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yjimk/smallbatch/types"
)

// counter is an atomic integer used for generating unique IDs for jobs.
var counter atomic.Int64

// Batcher provides a mechanism to group individual jobs into batches
// for more efficient processing. It allows the configuration of batch size
// and the frequency of batch processing.
//
// Fields:
// - config: Contains parameters for controlling the behavior of the batcher, such as batch size and processing interval.
// - jobs: A channel for receiving jobs to be batched and processed.
// - results: A mapping from job ID to a channel where the processed job result will be sent.
// - batchProcessor: An interface for the actual processing of batches. The implementation is provided by the user.
// - wg: A wait group to keep track of ongoing jobs and ensure graceful shutdown.
// - mu: A mutex to safeguard concurrent access to the results map.
// - resultChanPool: A pool of channels to optimise the allocation and reuse of result channels.
// - currentBatchNumber: A counter to keep track of the batches that have been processed.
// - shutdown: A channel used to signal the batcher to stop processing and shut down.
// - shutdownOnce: Ensures that the shutdown process is executed only once.
// - closed: An atomic value to indicate whether the batcher has been closed or not, aiding in thread-safe operations.
type Batcher struct {
	config             Config
	jobs               chan types.Job
	results            map[string]chan types.JobResult
	batchProcessor     types.BatchProcessor
	wg                 sync.WaitGroup
	mu                 sync.Mutex
	resultChanPool     sync.Pool
	currentBatchNumber atomic.Int64
	shutdown           chan struct{}
	shutdownOnce       *sync.Once
	closed             atomic.Value
}

// NewBatcher initialises and returns a new Batcher instance.
// It sets up the necessary channels, maps, and configuration based on the provided
// types.BatchProcessor and Config.
func NewBatcher(bp types.BatchProcessor, config Config) *Batcher {
	b := &Batcher{
		config:         config,
		jobs:           make(chan types.Job, config.BatchSize),
		results:        make(map[string]chan types.JobResult, config.BatchSize),
		shutdown:       make(chan struct{}),
		batchProcessor: bp,
		shutdownOnce:   &sync.Once{},
	}

	// Initialise the result channel pool
	b.resultChanPool.New = func() interface{} {
		return make(chan types.JobResult, 1)
	}
	b.closed.Store(false) // Initialise as not closed.

	return b
}

// SubmitJob accepts a job for processing, waits for the job to be processed,
// and returns the result. If the job doesn't have an ID, one will be generated.
// It internally uses a channel pool to manage result channels.
func (b *Batcher) SubmitJob(job types.Job) types.JobResult {
	// Check if Batcher is closed and return an error if so
	if b.closed.Load().(bool) {
		return types.JobResult{
			Err: errors.New("batcher is closed"),
		}
	}

	b.wg.Add(1)

	// If the job doesn't have an ID, use the counter
	if job.ID == "" {
		job.ID = fmt.Sprintf("%d", counter.Add(1)) // This should potentially be a UUID instead, but wanted to avoid the dependency.
	}

	// Retrieve a result channel from the pool. This will be used to receive the
	// result of the job once it has been processed.
	resultChan := b.resultChanPool.Get().(chan types.JobResult)

	// Map the job ID to its corresponding result channel so the processing
	// function knows where to send the result.
	b.mu.Lock()
	b.results[job.ID] = resultChan
	b.mu.Unlock()

	// Submit the job to the channel for processing.
	b.jobs <- job

	// Block and wait for the result of the job.
	result := <-resultChan

	// Return the channel to the pool for reuse.
	b.resultChanPool.Put(resultChan)

	return result
}

// Initiates the processing loop of the Batcher.
// It collects jobs into batches and processes them either when:
// - The batch size reaches the configured limit or
// - The configured time interval elapses, whichever happens first.
func (b *Batcher) Start() {
	// Initialise a ticker based on the configured interval to trigger batch processing.
	ticker := time.NewTicker(b.config.TimeInterval)
	defer ticker.Stop()

	// Create a slice to store jobs for the current batch.
	currentBatch := make([]types.Job, 0, b.config.BatchSize)

	for {
		// Loop to collect jobs up to the BatchSize.
		for len(currentBatch) < b.config.BatchSize {
			select {
			case job, ok := <-b.jobs:
				if !ok {
					// Exit the loop if the jobs channel is closed.
					return
				}
				// If a job is received, append it to the current batch.
				currentBatch = append(currentBatch, job)
			case <-ticker.C:
				// If the ticker fires before the batch size is reached,
				// process the current batch and then reset it.
				b.processBatch(currentBatch)
				currentBatch = currentBatch[:0] // Retain the underlying memory but reset the slice
				break
			case <-b.shutdown:
				// If a shutdown request is received, process any jobs left in the batch and then exit.
				if len(currentBatch) > 0 {
					b.processBatch(currentBatch)
				}
				return
			}
		}

		// At this point, the batch is full. Process the jobs.
		if len(currentBatch) == b.config.BatchSize {
			b.processBatch(currentBatch)
			currentBatch = currentBatch[:0] // Retain the underlying memory but reset the slice.
		}

		// Wait for the next ticker or shutdown signal.
		select {
		case <-ticker.C:
			// Continue to the outer loop to collect more jobs.
		case <-b.shutdown:
			// If a shutdown request is received, exit the loop.
			return
		}
	}
}

// processBatch takes a batch of jobs, processes them concurrently using the provided batchProcessor
//
// Parameters:
// - batch: A slice of jobs that need to be processed.
func (b *Batcher) processBatch(batch []types.Job) {
	// Determine the current batch number and increment it for subsequent batches.
	batchNumber := b.currentBatchNumber.Load()
	b.currentBatchNumber.Add(1)

	// Process each job in the batch concurrently.
	for _, job := range batch {
		go func(j types.Job) {
			// Use the batchProcessor to process the job.
			result := b.batchProcessor.Process(j)

			// Attach relevant metadata to the result.
			result.JobID = j.ID
			result.BatchNumber = batchNumber

			// If there's a corresponding result channel for the job, send the result.
			b.mu.Lock()
			if ch, ok := b.results[result.JobID]; ok {
				ch <- result
				// Remove the used result channel from the map.
				delete(b.results, result.JobID)
			}
			b.mu.Unlock()

			b.wg.Done()
		}(job)
	}
}

// Shutdown gracefully shuts down the Batcher. It signals the internal components to stop accepting
// new jobs and waits for the processing of the current batch of jobs to complete.
// This method can be called multiple times, but it will only have an effect once.
func (b *Batcher) Shutdown() <-chan struct{} {
	var done chan struct{}

	// Ensure that shutdown is only called once
	b.shutdownOnce.Do(func() {
		done = make(chan struct{})
		b.closed.Store(true) // Mark the batcher as closed.
		close(b.jobs)        // Close the jobs channel so no more jobs can be submitted.

		go func() {
			b.wg.Wait()       // Wait for all ongoing jobs to finish.
			close(b.shutdown) // Signal the batcher to shut down.
			close(done)       // Notify that shutdown is complete.
		}()
	})

	return done
}
