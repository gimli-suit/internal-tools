BINARY = pd-slack-sync

.PHONY: build run install test clean

clean:
	rm -f $(BINARY)

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

install:
	go install .

test:
	go test ./...
