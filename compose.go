package confort

import (
	"context"
	"fmt"

	"github.com/daichitakahashi/confort/compose"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/docker/client"
	"github.com/lestrrat-go/option"
)

type ComposeProject struct {
	composer compose.Composer
	cli      *client.Client
	ex       exclusion.Control
}

type (
	composeIdent  interface{ compose() }
	ComposeOption interface {
		option.Interface
		composeIdent
	}
	// TODO: identOptionProjectDir  struct{}
	// TODO: identOptionProjectName struct{}
	identOptionComposeBackend struct{}
	composeOption             struct {
		option.Interface
		composeIdent
	}
)

func WithComposeBackend(b compose.Backend) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionComposeBackend{}, b),
	}
}

func Compose(ctx context.Context, configFiles []string, opts ...ComposeOption) (*ComposeProject, error) {
	var (
		be         compose.Backend = &composeBackend{}
		clientOpts                 = []client.Opt{
			client.FromEnv,
		}
	)

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionComposeBackend{}:
			be = opt.Value().(compose.Backend)
		}
	}

	// create docker API client
	apiClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, err
	}
	apiClient.NegotiateAPIVersion(ctx)

	composer, err := be.Load(ctx, "", configFiles, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("compose: %w", err)
	}

	return &ComposeProject{
		composer: composer,
		cli:      apiClient,
	}, nil
}

func (c *ComposeProject) Close() error {
	return nil
}

type (
	upIdent  interface{ up() }
	UpOption interface {
		option.Interface
		upIdent
	}
	identOptionWaiter struct{}
	upOption          struct {
		option.Interface
		upIdent
	}
)

func WithWaiter(w *wait.Waiter) UpOption {
	return upOption{
		Interface: option.New(identOptionWaiter{}, w),
	}
}

type Service struct {
	c            *ComposeProject
	name         string
	image        string
	containerIDs []string
	env          map[string]string
	ports        Ports
	waiter       *wait.Waiter
}

func (c *ComposeProject) Up(ctx context.Context, service string, opts ...UpOption) (*Service, error) {
	svc, err := c.composer.Up(ctx, service, compose.UpOptions{
		Scale:  0,
		Waiter: nil,
	})
	if err != nil {
		return nil, err
	}

	ports := Ports{}
	for _, id := range svc.ContainerIDs {
		info, err := c.cli.ContainerInspect(ctx, id)
		if err != nil {
			return nil, err
		}
		// merge ports
		for cp, b := range info.NetworkSettings.Ports {
			if len(b) == 0 {
				continue
			}
			ports[cp] = append(ports[cp], b...)
		}
	}

	return nil, nil
}

func (s *Service) Use(ctx context.Context, exclusive bool, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, exclusive, opts...)
}

func (s *Service) containerIdent() string {
	return s.name
}

func (s *Service) containerPorts() Ports {
	return s.ports
}

func (s *Service) exclusionControl() exclusion.Control {
	return s.c.ex
}

func (s *Service) UseExclusive(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, true, opts...)
}

func (s *Service) UseShared(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, false, opts...)
}
