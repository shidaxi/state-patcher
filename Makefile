BINARY_NAME := state-patcher
MODULE := github.com/shidaxi/state-patcher

.PHONY: build clean snapshot

build:
	go build -trimpath -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

snapshot:
	goreleaser release --snapshot --clean
