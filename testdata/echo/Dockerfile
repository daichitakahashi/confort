FROM golang:1.18.2-alpine3.15 AS builder
WORKDIR /go/src
COPY . /go/src
RUN go build -o echo .

FROM alpine:3.15.4
EXPOSE 80
HEALTHCHECK --interval=1s --timeout=3s CMD wget -q -O - http://localhost/ || exit 1
COPY --from=builder /go/src/echo /usr/local/
ENTRYPOINT /usr/local/echo
