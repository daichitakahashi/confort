//go:generate go run github.com/matryer/moq@v0.3.0 -pkg mock -out fetcher_moq.gen.go ../../wait Fetcher:Fetcher

package mock
