# Change variables as necessary.
BINARY_NAME :=  popsocket
PACKAGE_PATH := ./cmd/popsocket/main.go

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

#=================#
# QUALITY CONTROL #
#=================#

.PHONY: tidy
tidy:
	gofumpt -l -d .
	go mod tidy -v
