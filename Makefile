test:
	go test ./...

build-cli:
	go build -o bin/crypto-taxes cli/cli.go
