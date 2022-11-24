build:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"

check:
	# go install github.com/kisielk/errcheck@latest
	errcheck -exclude errcheck_excludes.txt ./...
	# go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck ./...
	go vet ./...

clean:
	go clean

test:
	go test ./...
