APP := jardec
override VERSION := $(shell tr -d '\r\n' < VERSION)

.PHONY: test build run clean install

test:
	go test ./...

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(APP) ./cmd/jardec

install:
	go install ./cmd/jardec

run:
	go run ./cmd/jardec --help

clean:
	rm -rf bin/ out/
