test:
	go test ./...

build-cli:
	go build -o bin/crypto-taxes cli/cli.go

build-release:
	GOOS=linux GOARCH=amd64 go build -o bin/crypto-taxes-linux-amd64 cli/cli.go
	GOOS=linux GOARCH=386 go build -o bin/crypto-taxes-linux-386 cli/cli.go
	GOOS=linux GOARCH=arm64 go build -o bin/crypto-taxes-linux-arm64 cli/cli.go
	GOOS=darwin GOARCH=amd64 go build -o bin/crypto-taxes-darwin-amd64 cli/cli.go
