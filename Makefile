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
lint: ## Run golangci-lint if installed, otherwise go vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, falling back to go vet"; \
		go vet ./...; \
	fi

.PHONY: nilaway
nilaway: ## Look for possible nil pointer panics
	go run go.uber.org/nilaway/cmd/nilaway@latest \
		-include-pkgs=github.com/pablitovicente/mqtt-load-generator ./...

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
