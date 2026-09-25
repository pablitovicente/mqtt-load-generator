BIN    := bin/mqtt-load-generator
IMAGE  := mqtt-load-generator:latest
GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: build
build: ## Build the binary into ./bin (override GOOS/GOARCH to cross-compile)
	mkdir -p bin
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BIN) ./cmd

.PHONY: test
test: ## Run tests with race detection
	go test -race ./...

.PHONY: lint
lint: ## Run golangci-lint (needs golangci-lint installed)
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found. Install it with:"; \
		echo "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
		exit 1; \
	fi
	golangci-lint run ./...

.PHONY: nilaway
nilaway: ## Look for possible nil pointer panics (needs nilaway installed)
	@if ! command -v nilaway >/dev/null 2>&1; then \
		echo "nilaway not found. Install it with:"; \
		echo "  go install go.uber.org/nilaway/cmd/nilaway@latest"; \
		exit 1; \
	fi
	nilaway -include-pkgs=github.com/pablitovicente/mqtt-load-generator ./...

.PHONY: check
check: lint nilaway test ## Everything to run before pushing

.PHONY: tidy
tidy: ## Sync go.mod/go.sum with the source
	go mod tidy

.PHONY: docker
docker: ## Build the Docker image
	docker build -t $(IMAGE) .

.PHONY: clean
clean: ## Remove build output
	rm -rf bin
