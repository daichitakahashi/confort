FROM golang:1.19-alpine3.16 AS builder
WORKDIR /go/src
COPY . /go/src
RUN go build -o communicator .

FROM alpine:3.16.2
ENV CM_TARGET=""
EXPOSE 80
HEALTHCHECK --interval=1s --timeout=3s CMD wget -q -O - http://localhost/get || exit 1
COPY --from=builder /go/src/communicator /usr/local/
ENTRYPOINT /usr/local/communicator
