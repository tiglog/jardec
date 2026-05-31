APP := jardec

.PHONY: test build run clean

test:
	go test ./...

build:
	go build -o bin/$(APP) ./cmd/jardec

run:
	go run ./cmd/jardec --help

clean:
	rm -rf bin/ out/
