proto:
	protoc -I./proto/beacon --go_out=. --go-grpc_out=. ./proto/beacon/*.proto

.PHONY: proto
