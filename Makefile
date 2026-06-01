APP := chatdock

.PHONY: run build test vet fmt fmt-check check clean

run:
	go run ./cmd/chatdock

build:
	mkdir -p bin
	go build -buildvcs=false -o bin/$(APP) ./cmd/chatdock

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l cmd internal)"

check: fmt-check vet test build

fmt:
	gofmt -w cmd internal

clean:
	rm -rf bin
