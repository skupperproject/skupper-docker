FROM golang:1.13

WORKDIR /go/src/app
COPY . .

RUN go mod download

RUN go build -o controller cmd/skupper-docker-controller/main.go cmd/skupper-docker-controller/controller.go cmd/skupper-docker-controller/service_sync.go

CMD ["/go/src/app/controller"]
