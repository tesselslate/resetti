build:
	mkdir -p out
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o out/resetti -ldflags="-s -w" ${GOFLAGS}
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o out/bench -ldflags="-s -w" ${GOFLAGS} ./contrib/bench.go

check:
	# go install github.com/kisielk/errcheck@latest
	errcheck -exclude errcheck_excludes.txt ./...
	# go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck ./...
	go vet ./...

deb: GOFLAGS=-ldflags="-X res.overrideDataDir=/usr/share/resetti"
deb: build
	mkdir -p out/deb/usr/local/bin
	mkdir -p out/deb/usr/local/share/resetti
	mkdir -p out/deb/DEBIAN
	cp .pkg/debian out/deb/DEBIAN/control
	@if git describe --exact-match HEAD; then 												\
		sed -i "s/VERSION/$$(cat ../.version)/" out/deb/DEBIAN/control; 					\
	else 																					\
		sed -i "s/VERSION/0.0.0dev-$$(git rev-parse --short HEAD)/" out/deb/DEBIAN/control; \
	fi
	cp out/bench out/deb/usr/local/bin
	cp out/resetti out/deb/usr/local/bin
	cp internal/res/{cgroup_setup.sh,default.toml,scene-setup.lua} out/deb/usr/local/share/resetti
	dpkg-deb --build --root-owner-group out/deb out/resetti.deb

clean:
	rm -r out
	go clean

test:
	go test ./...
