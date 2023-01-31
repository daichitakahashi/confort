package compose

import (
	"context"
)

type (
	Backend interface {
		// TODO: ResourcePolicy
		Load(ctx context.Context, projectDir string, configFiles, profiles []string, envFile string) (Composer, error)
	}

	Composer interface {
		ProjectName() string
		Up(ctx context.Context, service string, opts UpOptions) (*Service, error)
		RemoveCreated(ctx context.Context, opts RemoveOptions) error
		Down(ctx context.Context, opts DownOptions) error
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
	UpOptions struct {
		Scale           int
		ScalingStrategy ScalingStrategy
	}

	RemoveOptions struct {
		RemoveAnonymousVolumes bool
	}

	DownOptions struct {
		RemoveOrphans bool
		RemoveVolumes bool
	}

	Service struct {
		Name         string
		ContainerIDs []string
		Env          map[string]string
	}
)

type ScalingStrategy int

const (
	ScalingStrategyScaleOut ScalingStrategy = iota
	ScalingStrategyConsistent
)
