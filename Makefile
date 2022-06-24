proto:
	protoc -I./proto/beacon --go_out=. --go-grpc_out=. ./proto/beacon/*.proto

test-cov:
	go test -coverprofile=coverage.out -coverpkg=./... ./...
	go tool cover -html=coverage.out
	rm coverage.out

.PHONY: proto test-cov
