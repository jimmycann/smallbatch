package smallbatch

import (
	"time"
)

// Config defines the configuration parameters for the Batcher.
// It allows users to specify the maximum number of tasks that
// can be grouped together in a batch (BatchSize) and the maximum
// duration to wait before processing a batch (TimeInterval).
type Config struct {
	BatchSize    int           // Maximum number of tasks in a single batch.
	TimeInterval time.Duration // Duration to wait before processing a batch.
}
