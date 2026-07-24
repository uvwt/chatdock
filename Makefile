APP := chatdock
WEB_DIR := web
DEV_COMPOSE := compose.dev.yaml
PROD_CHATDOCK_DIR ?= /Volumes/KIOXIA/Docker/chatdock

.PHONY: run build web-deps web-build web-dev js-check css-check bundle-check frontend-test frontend-lint backend-lint commit-msg-check shell-check test vet fmt fmt-check check clean dev-up dev-down prod-check prod-health deploy-prod

run: web-build
	go run ./cmd/chatdock

build: web-build
	mkdir -p bin
	go build -buildvcs=false -o bin/$(APP) ./cmd/chatdock

web-deps:
	@if ! npm --prefix $(WEB_DIR) ls --depth=0 >/dev/null 2>&1; then \
		if [ -f $(WEB_DIR)/package-lock.json ]; then \
			npm --prefix $(WEB_DIR) ci; \
		else \
			npm --prefix $(WEB_DIR) install; \
		fi; \
	fi

web-build: web-deps
	npm --prefix $(WEB_DIR) run build

web-dev: web-deps
	npm --prefix $(WEB_DIR) run dev

js-check: web-deps
	node --check $(WEB_DIR)/vite.config.js
	@for f in $(WEB_DIR)/src/lib/*.js $(WEB_DIR)/src/lib/*.mjs $(WEB_DIR)/src/hooks/*.js; do [ -f $$f ] && node --check $$f; done

css-check:
	node scripts/check-css-health.mjs

bundle-check: web-build
	node scripts/check-bundle-size.mjs

frontend-test: web-deps
	npm --prefix $(WEB_DIR) test

frontend-lint: js-check css-check
	node scripts/check-frontend.mjs

backend-lint:
	scripts/check-backend-structure.sh

commit-msg-check:
	@test -n "$(MSG)" || (echo 'usage: make commit-msg-check MSG="refactor(chatdock): 中文说明"' >&2; exit 2)
	node scripts/check-commit-message.mjs --message "$(MSG)"

shell-check:
	@for file in scripts/*.sh; do bash -n "$$file"; done

test: web-build
	go test ./...

vet: web-build
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l cmd internal web/*.go)"

check: fmt-check shell-check backend-lint frontend-lint frontend-test bundle-check vet test build

fmt:
	gofmt -w cmd internal web/*.go

dev-up:
	docker compose -f $(DEV_COMPOSE) up -d --build

dev-down:
	docker compose -f $(DEV_COMPOSE) down

prod-check:
	CHATDOCK_PROD_DIR=$(PROD_CHATDOCK_DIR) scripts/check-prod-compose.sh

prod-health:
	scripts/check-prod-health.sh

deploy-prod:
	cd $(PROD_CHATDOCK_DIR) && CHATDOCK_PROD_DIR=$(PROD_CHATDOCK_DIR) $(CURDIR)/scripts/deploy-prod.sh

clean:
	rm -rf bin $(WEB_DIR)/dist
