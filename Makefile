APP := chatdock
WEB_DIR := web

.PHONY: run build web-deps web-build js-check test vet fmt fmt-check check clean

run: web-build
	go run ./cmd/chatdock

build: web-build
	mkdir -p bin
	go build -buildvcs=false -o bin/$(APP) ./cmd/chatdock

web-deps:
	@if [ ! -d $(WEB_DIR)/node_modules ]; then \
		if [ -f $(WEB_DIR)/package-lock.json ]; then \
			npm --prefix $(WEB_DIR) ci; \
		else \
			npm --prefix $(WEB_DIR) install; \
		fi; \
	fi

web-build: web-deps
	npm --prefix $(WEB_DIR) run build

js-check: web-deps
	node --check $(WEB_DIR)/src/main.js
	@for f in $(WEB_DIR)/src/modules/*.js; do node --check "$$f" || exit 1; done
	node scripts/check-actions.js

test: web-build
	go test ./...

vet: web-build
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l cmd internal web/*.go)"

check: fmt-check js-check vet test build

fmt:
	gofmt -w cmd internal web/*.go

clean:
	rm -rf bin $(WEB_DIR)/dist
