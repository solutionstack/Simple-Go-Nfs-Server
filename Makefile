build:
	@go build -o bin/nfs

test:
	@go test ./... -v

tidy:
	@go mod tidy

