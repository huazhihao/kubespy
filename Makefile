.PHONY: build clean test

GIT_SHA=`git rev-parse --short HEAD || echo`

build:
	@echo "Building .."
	@mkdir -p bin
	@go build -ldflags "-X main.GitSHA=${GIT_SHA}" -o bin/kubespy .

clean:
	@echo "Cleaning .."
	@rm -f bin/*

test:
	@echo "Running tests .."
	@go test -v

check:
	@echo "Checking .."
	@go vet -v ./...

