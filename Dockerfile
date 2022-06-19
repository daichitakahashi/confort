FROM golang:1.18.3-alpine AS builder

COPY go.mod go.sum /go/src/
COPY ./beacon/go.mod ./beacon/go.sum /go/src/beacon/
WORKDIR /go/src/beacon
RUN go mod download

COPY . /go/src
RUN go build -o /go/bin/beacon .

FROM alpine:3.16.0

RUN apk add docker && GRPC_HEALTH_PROBE_VERSION=v0.4.11 && \
    wget -qO/bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /bin/grpc_health_probe
COPY --from=builder /go/bin/beacon /bin

EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s CMD grpc_health_probe -addr 127.0.0.1:8080

ENTRYPOINT ["/bin/beacon"]
