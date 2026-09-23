# Change variables as necessary.
BINARY_NAME :=  popsocket
PACKAGE_PATH := ./cmd/popsocket/main.go
GOLANGCI_LINT_VERSION := v2.13.2

#=============#
# DEVELOPMENT #
#=============#

.PHONY: build
build:
	go build -o ${BINARY_NAME} ${PACKAGE_PATH}

.PHONY: test
test:
	go test -v -race -buildvcs ./...

.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

.PHONY: docker-image
docker-image:
	docker build -t sonastea/popsocket:latest .

#=================#
# QUALITY CONTROL #
#=================#

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: lint/install
lint/install:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: tidy
tidy:
	gofumpt -l -d .
	go mod tidy -v
