package main

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/yjimk/smallbatch"
	"github.com/yjimk/smallbatch/types"
)

type FizzBuzzProcessor struct{}

func (dbp *FizzBuzzProcessor) Process(job types.Job) types.JobResult {
	payload, ok := job.Payload.(int)
	if !ok {
		return types.JobResult{
			Output: nil,
			Err:    fmt.Errorf("invalid job payload type, expected int"),
		}
	}

	var output string
	switch {
	case payload%3 == 0 && payload%5 == 0:
		output = "FizzBuzz"
	case payload%3 == 0:
		output = "Fizz"
	case payload%5 == 0:
		output = "Buzz"
	default:
		output = strconv.Itoa(payload)
	}

	return types.JobResult{
		Output: fmt.Sprintf("%d = %s", payload, output),
		Err:    nil,
	}
}

func main() {
	var wg sync.WaitGroup // Define a WaitGroup
	// initialise the Batcher
	batcher := smallbatch.NewBatcher(&FizzBuzzProcessor{}, smallbatch.Config{
		BatchSize:    5,
		TimeInterval: 1 * time.Second,
	})

	// Start the Batcher
	go batcher.Start()

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

	wg.Wait() // Wait for all submitted jobs to finish

	// Shutdown the Batcher
	<-batcher.Shutdown() // This will block until shutdown is complete

	fmt.Println("done")
}
