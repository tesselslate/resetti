build:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"

check:
	errcheck ./...		# go install github.com/kisielk/errcheck@latest
	staticcheck ./...	# go install honnef.co/go/tools/cmd/staticcheck@latest
	go vet ./...

clean:
	go clean

test:
	go test ./...
