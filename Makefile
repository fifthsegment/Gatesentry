.PHONY: test build run clean-test lint lint-security tests

clean-test:
	@echo "Cleaning up test artifacts..."
	@-kill `cat /tmp/gatesentry.pid 2>/dev/null` 2>/dev/null || true
	@-rm -f /tmp/gatesentry.pid /tmp/gatesentry.log
	@-rm -rf /tmp/gatesentry

# Run all linters (golangci-lint + shellcheck)
lint:
	@echo "=== Linting application module ==="
	golangci-lint run ./application/...
	@echo "=== Linting gatesentryproxy module ==="
	golangci-lint run ./gatesentryproxy/...
	@echo "=== Linting root module ==="
	golangci-lint run .
	@echo "=== Linting shell scripts ==="
	shellcheck -S warning build.sh run.sh restart.sh docker-publish.sh scripts/*.sh || true
	@echo "=== All linting complete ==="

# Run only security-focused linters (fast, good pre-commit check)
lint-security:
	@echo "=== Security scan (gosec) ==="
	golangci-lint run --enable-only gosec ./application/... ./gatesentryproxy/... .
	@echo "=== Secret detection (gitleaks) ==="
	gitleaks git --staged --verbose || true

# Run all unit tests across all modules
tests:
	@echo "=== Running unit tests ==="
	cd application && go test -v -count=1 ./...
	cd gatesentryproxy && go test -v -count=1 ./...
	go test -v -count=1 ./...

build: clean-test
	go build -o /tmp/gatesentry-bin .

run: build
	cd /tmp && ./gatesentry-bin

# Run integration tests with server
test: clean-test build
	@echo "Starting Gatesentry server in background..."
	@cd /tmp && GS_ADMIN_PORT=8080 ./gatesentry-bin > /tmp/gatesentry.log 2>&1 & echo $$! > /tmp/gatesentry.pid
	@echo "Waiting for server to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		if curl -s http://localhost:8080/gatesentry/api/about > /dev/null 2>&1; then \
			echo "Server is ready!"; \
			break; \
		fi; \
		if [ $$i -eq 15 ]; then \
			echo "Server failed to start. Logs:"; \
			cat /tmp/gatesentry.log; \
			kill `cat /tmp/gatesentry.pid` 2>/dev/null || true; \
			rm -f /tmp/gatesentry.pid; \
			exit 1; \
		fi; \
		echo "Waiting... ($$i/15)"; \
		sleep 2; \
	done
	@echo "Running tests..."
	@GS_ADMIN_PORT=8080 GODEBUG=gotestcache=off go test -v -timeout 5m ./tests/... -coverprofile=coverage.txt -covermode=atomic || (kill `cat /tmp/gatesentry.pid` 2>/dev/null; rm -f /tmp/gatesentry.pid; exit 1)
	@echo "Stopping server..."
	@kill `cat /tmp/gatesentry.pid` 2>/dev/null || true
	@rm -f /tmp/gatesentry.pid
