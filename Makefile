.PHONY: build test_non_ui run_tests run_client_a run_client_b run_clients

build:
	mkdir -p ./bin
	go build -o ./bin/gosend .

test_non_ui:
	@pkgs=$$(GOCACHE=/tmp/go-build go list ./... | grep -Ev '^gosend$$|/ui$$'); \
	if [ -z "$$pkgs" ]; then \
		echo "No non-UI Go packages found to test."; \
		exit 1; \
	fi; \
	GOCACHE=/tmp/go-build go test $$pkgs

run_client_a: build
	P2P_CHAT_DATA_DIR=/tmp/gosend-a ./bin/gosend

run_client_b:
	P2P_CHAT_DATA_DIR=/tmp/gosend-b ./bin/gosend

run_clients: build
	rm -rf /tmp/gosend-*
	(P2P_CHAT_DATA_DIR=/tmp/gosend-a ./bin/gosend & P2P_CHAT_DATA_DIR=/tmp/gosend-b ./bin/gosend & wait)
