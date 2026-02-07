.PHONY: build run_tests run_client_a run_client_b

build:
	mkdir -p ./bin
	go build -o ./bin/gosend .

run_client_a: build
	P2P_CHAT_DATA_DIR=/tmp/gosend-a ./bin/gosend

run_client_b:
	P2P_CHAT_DATA_DIR=/tmp/gosend-b ./bin/gosend
