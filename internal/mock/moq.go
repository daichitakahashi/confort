//go:generate go run github.com/matryer/moq@v0.2.7 -pkg mock -out fetcher_moq.go ../../ Fetcher
//go:generate go run github.com/matryer/moq@v0.2.7 -pkg mock -out backend_moq.go ../../ Backend Namespace

package mock
