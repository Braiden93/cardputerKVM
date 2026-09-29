.PHONY: build run clean tidy

BINARY_NAME=cardputer-kvm

build:
	go build -ldflags="-s -w" -o $(BINARY_NAME)

run: build
	./$(BINARY_NAME)

tidy:
	go mod tidy

clean:
	rm -f $(BINARY_NAME)
