//go:generate go run github.com/matryer/moq@v0.2.7 -pkg mock -out fetcher_moq.go ../../ Fetcher
//go:generate go run github.com/matryer/moq@v0.2.7 -pkg mock -out exclusion_control_moq.go ../../ ExclusionControl:ExclusionControl

package mock
