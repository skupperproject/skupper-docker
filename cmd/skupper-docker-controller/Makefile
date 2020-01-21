IMAGE := quay.io/skupper/skupper-docker-proxy-controller

.PHONY: build
build:
	docker build -t ${IMAGE} .

.PHONY: run
run: build
	docker run -p 8080:8081 ${IMAGE}

# Prerequisite: docker login quay.io
.PHONY: push
push: build
	docker push ${IMAGE}
