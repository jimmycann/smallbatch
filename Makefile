# Paths
PKGS = $(shell go list ./...)
MOCKGEN = mockgen

# If the tool is missing, go get it
$(MOCKGEN):
	go get github.com/golang/mock/gomock
	go install github.com/golang/mock/mockgen@latest

# Generate mocks for BatchProcessor
.PHONY: generate
generate: $(MOCKGEN)
	go generate ./...

# All setup tasks (e.g., install tools)
.PHONY: setup
setup: $(MOCKGEN)
	@echo "Setup complete."

# Run tests
.PHONY: test
test:
	go test $(PKGS)

# Run all
.PHONY: all
all: setup generate test
