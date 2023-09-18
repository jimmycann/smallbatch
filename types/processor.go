package types

// BatchProcessor defines an interface for processing individual
// jobs. Implementers of this interface provide the logic for
// processing each job and returning the results.
//
//go:generate mockgen -source=processor.go -destination=../mocks/mock_processor.go
type BatchProcessor interface {
	Process(Job) JobResult
}
