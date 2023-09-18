package types

import "context"

// Job represents a unit of work to be processed
type Job struct {
	Ctx     context.Context // Context associated with the job, allowing for cancellation, timeout, etc.
	ID      string          // Unique identifier for the job.
	Payload interface{}     // Content or data of the job for processing.
}

// JobResult captures the result of processing a Job
type JobResult struct {
	JobID       string      // Identifier for the processed job.
	BatchNumber int64       // Batch in which the job was processed.
	Output      interface{} // Result from processing the job.
	Err         error       // Any error encountered during processing.
}
