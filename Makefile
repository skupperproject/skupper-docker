VERSION := $(shell git describe --tags --dirty=-modified)
IMAGE := quay.io/skupper/skupper-docker-controller

all: build-cmd build-controller

build-cmd:
	go build -ldflags="-X main.version=${VERSION}"  -o skupper-docker cmd/skupper-docker-cli/main.go

build-controller:
	go build -ldflags="-X main.version=${VERSION}"  -o skupper-docker-controller cmd/skupper-docker-controller/main.go

docker-build:
	docker build -t ${IMAGE} .

docker-push:
	docker push ${IMAGE}

clean:
	rm -rf skupper-docker skupper-docker-controller release

package: release/windows.zip release/darwin.zip release/linux.tgz

release/linux.tgz: release/linux/skupper-docker
	tar -czf release/linux.tgz -C release/linux/ skupper-docker

release/linux/skupper-docker: cmd/skupper-docker-cli/main.go
	GOOS=linux GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/linux/skupper-docker cmd/skupper-docker-cli/main.go

release/windows/skupper-docker: cmd/skupper-docker-cli/main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/windows/skupper-docker cmd/skupper-docker-cli/main.go

release/windows.zip: release/windows/skupper-docker
	zip -j release/windows.zip release/windows/skupper-docker

release/darwin/skupper-docker: cmd/skupper-docker-cli/main.go
	GOOS=darwin GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/darwin/skupper-docker cmd/skupper-docker-cli/main.go

release/darwin.zip: release/darwin/skupper-docker
	zip -j release/darwin.zip release/darwin/skupper-docker
