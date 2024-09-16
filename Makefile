# Change variables as necessary.
BINARY_NAME :=  popsocket
PACKAGE_PATH := ./cmd/popsocket/main.go

#=============#
# DEVELOPMENT #
#=============#

.PHONY: build
build:
	go build -o ${BINARY_NAME} ${PACKAGE_PATH}


#=================#
# QUALITY CONTROL #
#=================#

.PHONY: tidy
tidy:
	gofumpt -l -d .
	go mod tidy -v
