proto:
	protoc -I./proto/beacon --go_out=. --go-grpc_out=. ./proto/beacon/*.proto

test-cov:
	go test -coverprofile=coverage.out.tmp -coverpkg=./... ./...
	cat coverage.out.tmp | grep -v ".pb." | grep -v ".gen." | grep -v ".testutil." > coverage.out
	go tool cover -func=coverage.out
	rm coverage.out

test-cov-visual:
	go test -coverprofile=coverage.out.tmp -coverpkg=./... ./...
	cat coverage.out.tmp | grep -v ".pb." | grep -v ".gen." | grep -v ".testutil." > coverage.out
	go tool cover -html=coverage.out
	rm coverage.out

.PHONY: proto test-cov
