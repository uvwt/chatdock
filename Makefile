APP := chatdock

.PHONY: run build test fmt clean

run:
	go run ./cmd/chatdock

build:
	mkdir -p bin
	go build -buildvcs=false -o bin/$(APP) ./cmd/chatdock

test:
	go test ./...

fmt:
	gofmt -w cmd internal

clean:
	rm -rf bin
