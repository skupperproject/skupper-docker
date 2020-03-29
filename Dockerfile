FROM golang:1.13

WORKDIR /go/src/app
COPY . .

RUN go mod download

RUN go build -o controller cmd/skupper-docker-controller/main.go

CMD ["/go/src/app/controller"]
