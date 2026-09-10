APP := jardec

.PHONY: test build run clean install

test:
	go test ./...

build:
	go build -o bin/$(APP) ./cmd/jardec

install:
	go install ./cmd/jardec

run:
	go run ./cmd/jardec --help

clean:
	rm -rf bin/ out/
