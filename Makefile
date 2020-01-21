VERSION := $(shell git describe --tags --dirty=-modified)

all: build

build:
	go build -ldflags="-X main.version=${VERSION}"  -o skupper-docker cmd/skupper-docker-cli/main.go

clean:
	rm -rf skupper-docker release

deps:
	dep ensure

package: release/windows.zip release/darwin.zip release/linux.tgz

release/linux.tgz: release/linux/skupper-docker
	tar -czf release/linux.tgz -C release/linux/ skupper-docker

release/linux/skupper-docker: cmd/skupper-docker-cli/main.go
	GOOS=linux GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/linux/skupper-docker cmd/skupper-docker-cli/main.go

release/windows/skupper-docker: cmd/skupper-docker-cli/main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/windows/skupper-docker cmd/skupper-docker-cli/main.go

release/windows.zip: release/windows/skupper-docker
	zip -j release/windows.zip release/windows/skupper-docker

release/darwin/skupper-docker: cmd/skupper-docker/main.go
	GOOS=darwin GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/darwin/skupper-docker cmd/skupper-docker-cli/main.go

release/darwin.zip: release/darwin/skupper-docker
	zip -j release/darwin.zip release/darwin/skupper-docker
