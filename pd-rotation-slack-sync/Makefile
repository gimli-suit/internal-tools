BINARY = pd-slack-sync

.PHONY: build run install test

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

install:
	go install .

test:
	go test ./...
