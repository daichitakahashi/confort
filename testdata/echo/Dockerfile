FROM golang:1.19-alpine3.16 AS builder
WORKDIR /go/src
COPY . /go/src
RUN go build -o echo .

FROM alpine:3.16.2
EXPOSE 80
HEALTHCHECK --interval=1s --timeout=3s CMD wget -q -O - http://localhost/ || exit 1
COPY --from=builder /go/src/echo /usr/local/
ENTRYPOINT /usr/local/echo
