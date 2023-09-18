# SmallBatch 🍺

[![GoDoc](https://godoc.org/github.com/yjimk/smallbatch?status.svg)](https://godoc.org/github.com/yjimk/smallbatch)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

SmallBatch is designed to help you spread a number of tasks into smaller batches with a configurable batch size and interval.

## Installation

```bash
go get github.com/yjimk/smallbatch@latest
```

## Usage

Also check out the `./examples` directory for more examples.

### Implement the BatchProcessor Interface

Before you start batching jobs, you need a way to process them. Implement the BatchProcessor interface's Process method with your specific processing logic:

```go
type MyProcessor struct{}

func (m *MyProcessor) Process(job types.Job) types.JobResult {
    // Your job processing logic here.
    // Return a JobResult after processing.
}
```

### Initialise the Batcher

Set your preferred configuration, batch size and processing frequency:

```go
processor := &MyProcessor{}
config := smallbatch.Config{
    BatchSize:     10,
    TimeInterval:  time.Second * 5, // process every 5 seconds
}
batcher := smallbatch.NewBatcher(processor, config)

// Start the Batcher
go batcher.Start()
```

### Submit Jobs & Receive Results

Now, you can submit jobs to the batcher and get the results:

```go
job := types.Job{
    ID:       "some-id", // if left empty, a unique ID will be generated
    Payload:  "Your job data", // This can be any type
}
result := batcher.SubmitJob(job)
// Handle the job result
if result.Err != nil {
    fmt.Println("Error processing job:", result.Err)
} else {
    fmt.Println("Processed job result:", result.Output)
}
```

Job submission is blocking by default. If you want to submit jobs asynchronously, use goroutines:

```go
var wg sync.WaitGroup

// Submit multiple jobs
for i := 1; i <= 10; i++ {
    wg.Add(1)
    go func(index int) {
        defer wg.Done()
        job := types.Job{
            Payload: index,
        }
        result := batcher.SubmitJob(job)
        if result.Err != nil {
            fmt.Printf("Error processing job %s: %s\n", result.JobID, result.Err)
        } else {
            fmt.Printf("Batch %d | Job %s: %s\n", result.BatchNumber, result.JobID, result.Output)
        }
    }(i)
}

wg.Wait() // Wait for job submission to complete
```

### Graceful Shutdown

The batcher continues waiting for jobs, so when you're done gracefully terminate the batcher:

```go
<-batcher.Shutdown() // This blocks until shutdown is complete
```

## Why?

The SmallBatch package offers a streamlined way to aggregate and process tasks in configurable batches, typically for reducing the number of concurrent requests to downstream systems. By allowing users to set batch size, frequency and processing function, it offers adaptability for diverse use-cases.

## Features

- Configurable Batching: Customize both the size of each batch and the frequency of batch processing.
- Pluggable Batch Processor: Seamlessly integrate with any batch processing logic.
- Graceful Shutdown: Ensures all ongoing jobs are completed before shutdown.

## Limitations

These are things that are not supported in the current version of SmallBatch:

- No retry logic for failed tasks
- No error threshold option. If a task fails it continues to process the remaining tasks and batches. This should be configurable so a user can bail out if a certain number of tasks fail.
- A slow task in the batch will delay processing of the remaining batches. This is a design decision to prioritise simplicity and flexibility. A user is able to pass context to a task so can implement their own logic to handle this with context.Timeout if required.
- Unable to pass logger to batch processor for debugging purposes
- No support for task priority in a batch
- Dynamic batch size. The batch size and interval is set when the batch processor is created, there should be the an option to allow the library to dynamically adjust the batch size and interval based on the number of tasks in the queue. Bit of a stretch but would be nice to have.
- A pause method to temporarily stop the batch processor. This could be useful if a user wants to pause the batch processor for a period of time and then resume it.
- Each job in the batch is being processed in its own goroutine. Depending on the batch size and the nature of the job, this could result in spawning a large number of goroutines in a very short period.
- This library allows setting of the job ID, which may be useful in some instances but could cause issues if the same ID is used for multiple jobs.

Ideally they would all be present/fixed but due to time and scope contraints they are not included in the current version.
