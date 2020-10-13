FROM golang:1.13 AS builder

WORKDIR /go/src/app
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .

RUN make build-controller

FROM registry.access.redhat.com/ubi8-minimal

WORKDIR /app
COPY --from=builder /go/src/app/controller .
CMD ["/app/controller"]
