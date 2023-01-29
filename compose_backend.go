package confort

import (
	"context"

	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/go-connections/nat"
)

type (
	ComposeBackend interface {
		Load(ctx context.Context, projectDir string, configFiles []string, envFile *string, profiles []string) (Composer, error)
	}
	Composer interface {
		// Up
		// *
		Up(ctx context.Context, service string, opts UpOptions) (Ports, error)
		Down(ctx context.Context, services []string) error
	}

	// UpOptions
	// --always-recreate-deps		Recreate dependent containers. Incompatible with --no-recreate.
	// --build		Build images before starting containers.
	// --force-recreate		Recreate containers even if their configuration and image haven't changed.
	// --no-deps		Don't start linked services.s
	// --no-recreate		If containers already exist, don't recreate them. Incompatible with --force-recreate.
	// --pull	missing	Pull image before running ("always"|"missing"|"never")
	// --remove-orphans		Remove containers for services not defined in the Compose file.
	// --renew-anon-volumes , -V		Recreate anonymous volumes instead of retrieving data from the previous containers.
	// --timeout , -t	10	Use this timeout in seconds for container shutdown when attached or when containers are already running.
	// --timestamps		Show timestamps.
	// --wait		Wait for services to be running|healthy. Implies detached mode.
	UpOptions struct {
		Scale  int
		Waiter *wait.Waiter
	}
)

type (
	// ComposeBackend implementation using Docker Compose CLI.
	composeBackend struct{}

	composer struct {
		proj projectConfig
	}

	projectConfig struct {
		Name        string
		ConfigFiles []string
		Services    []struct {
			// services that has been enabled by profiles
			Name         string
			ExposedPorts nat.PortMap
		}
	}
)

func (composeBackend) Load(ctx context.Context, projectDir string, configFiles []string, envFile *string, profiles []string) (Composer, error) {
	return &composer{}, nil
}

var _ ComposeBackend = (*composeBackend)(nil)

func (c composer) Up(ctx context.Context, service string, opts UpOptions) (Ports, error) {
	//TODO implement me
	panic("implement me")
}

func (c composer) Down(ctx context.Context, services []string) error {
	//TODO implement me
	panic("implement me")
}

var _ Composer = (*composer)(nil)
