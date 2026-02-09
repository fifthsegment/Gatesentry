.PHONY: test build run clean-test

clean-test:
	@echo "Cleaning up test artifacts..."
	@-kill `cat /tmp/gatesentry.pid 2>/dev/null` 2>/dev/null || true
	@-rm -f /tmp/gatesentry.pid /tmp/gatesentry.log
	@-rm -rf /tmp/gatesentry

build: clean-test
	go build -o /tmp/gatesentry-bin .

run: build
	cd /tmp && ./gatesentry-bin

# Run integration tests with server
test: clean-test build
	@echo "Starting Gatesentry server in background..."
	@cd /tmp && ./gatesentry-bin > /dev/null 2>&1 & echo $$! > /tmp/gatesentry.pid
	@echo "Waiting for server to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		if curl -s http://localhost:80/gatesentry/api/health > /dev/null 2>&1 || curl -s http://localhost:80/gatesentry/ > /dev/null 2>&1; then \
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
	@GODEBUG=gotestcache=off go test -v -timeout 5m ./tests/... -coverprofile=coverage.txt -covermode=atomic || (kill `cat /tmp/gatesentry.pid` 2>/dev/null; rm -f /tmp/gatesentry.pid; exit 1)
	@echo "Stopping server..."
	@kill `cat /tmp/gatesentry.pid` 2>/dev/null || true
	@rm -f /tmp/gatesentry.pid
