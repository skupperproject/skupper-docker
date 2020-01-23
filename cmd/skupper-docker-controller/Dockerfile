FROM golang:1.12

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN rm -rf /go/src/github.com/skupperproject/skupper-docker/vendor
RUN go install -v ./...

CMD ["app"]
