package confort

import (
	"context"

	"github.com/docker/docker/client"
	"github.com/lestrrat-go/option"
)

type ComposeProject struct {
	backend ComposeBackend
	cli     *client.Client
}

type (
	composeIdent  interface{ compose() }
	ComposeOption interface {
		option.Interface
		composeIdent
	}
	// TODO: identOptionProjectDir  struct{}
	// TODO: identOptionProjectName struct{}
	composeOption struct {
		option.Interface
		composeIdent
	}
)

func Compose(ctx context.Context, configFiles []string, opts ...ComposeOption) (*ComposeProject, error) {
	var (
		be         composeBackend
		clientOpts = []client.Opt{
			client.FromEnv,
		}
	)

	for _, opt := range opts {
		switch opt.Ident() {
		// TODO: check options
		}
	}

	// create docker API client
	apiClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, err
	}
	apiClient.NegotiateAPIVersion(ctx)

	return &ComposeProject{
		backend: be,
		cli:     apiClient,
	}, nil
}

func WithComposeBackend(b ComposeBackend) {}
