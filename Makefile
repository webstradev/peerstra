build:
	@go build -o bin/peerstra

run: build
	@bin/peerstra

test:
	@go test ./... -v --race