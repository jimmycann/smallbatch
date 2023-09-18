package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/yjimk/smallbatch"
	"github.com/yjimk/smallbatch/types"
)

type HTTPBatchProcessor struct{}

func (h *HTTPBatchProcessor) Process(job types.Job) types.JobResult {
	url, ok := job.Payload.(string)
	if !ok {
		return types.JobResult{
			Output: nil,
			Err:    fmt.Errorf("invalid URL payload"),
		}
	}

	timeout := time.Second * 2
	if url == "http://www.google.com" { // Simulate context cancellation just for this URL
		timeout = time.Millisecond * 1
	}

	ctx, cancel := context.WithTimeout(job.Ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.JobResult{
			Output: nil,
			Err:    err,
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.JobResult{
			Output: nil,
			Err:    err,
		}
	}
	defer resp.Body.Close()

	return types.JobResult{
		Output: fmt.Sprintf("%s - %d", url, resp.StatusCode),
		Err:    nil,
	}
}

func main() {
	urls := []string{
		"http://www.google.com",
		"http://www.example.com",
		"http://www.yahoo.com",
		"http://www.google.com",
		"http://www.example.com",
		"http://www.yahoo.com",
	}
	var wg sync.WaitGroup

	// initialise the Batcher
	batcher := smallbatch.NewBatcher(&HTTPBatchProcessor{}, smallbatch.Config{
		BatchSize:    4,
		TimeInterval: 2 * time.Second,
	})

	// Start the Batcher
	go batcher.Start()
	fmt.Println("Batcher started. Google will (deliberately) context cancel!")

	for i, url := range urls {
		wg.Add(1)

		go func(i int, url string) {
			defer wg.Done()

			ctx := context.Background()
			job := types.Job{
				Ctx:     ctx,
				ID:      strconv.Itoa(i),
				Payload: url,
			}
			result := batcher.SubmitJob(job)
			if result.Err != nil {
				fmt.Printf("Batch %d | Job %s: Error processing. %s\n", result.BatchNumber, job.ID, result.Err)
			} else {
				fmt.Printf("Batch %d | Job %s: %s\n", result.BatchNumber, job.ID, result.Output)
			}
		}(i, url)
	}

	wg.Wait() // Wait for all submitted goroutines to finish

	// Shutdown the Batcher
	fmt.Println("Batcher shutting down...")

	// Shutdown the Batcher
	<-batcher.Shutdown() // This will block until shutdown is complete

	fmt.Println("Batcher has shut down. Trying to submit another job should error")

	// Submit a single job, after the Batcher has been shutdown
	// This will return an error because the Batcher is closed
	job := types.Job{
		Payload: "https://jimmycann.com",
	}
	result := batcher.SubmitJob(job)
	if result.Err != nil {
		fmt.Printf("Error processing job: %s\n", result.Err)
	} else {
		fmt.Println("no way")
	}

	fmt.Println("done")
}
