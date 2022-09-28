proto:
	protoc -I./internal/beacon/proto --go_out=. --go-grpc_out=. ./internal/beacon/proto/*.proto

sqlc:
	go generate ./e2e/tenant/database

test:
	go test -coverprofile=coverage.out.tmp -p 1 -coverpkg=./... ./...
	cat coverage.out.tmp | grep -v ".pb." | grep -v ".gen." > coverage.out
	rm coverage.out.tmp

test-cov: test
	go tool cover -func=coverage.out
	rm coverage.out

test-cov-visual: test
	go tool cover -html=coverage.out
	rm coverage.out

test-cov-ci: test
	mkdir -p .cov
	go tool cover -html=coverage.out -o ./.cov/coverage.html
	go tool cover -func=coverage.out
	@export COVERAGE=$$(go tool cover -func=coverage.out | tail -n 1 | awk '{print $$3}') && \
	echo "{\
	\"schemaVersion\": 1,\
	\"label\": \"coverage\",\
	\"message\": \"$${COVERAGE}\",\
	\"color\": \"blue\"\
	}" > ./.cov/coverage.json

.PHONY: proto sqlc test test-cov test-cov-visual test-cov-ci
