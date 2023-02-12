package compose

import (
	"context"
)

type (
	Backend interface {
		Load(ctx context.Context, config string, opts LoadOptions) (Composer, error)
	}

	ResourcePolicy struct {
		AllowReuse bool
		Remove     bool
		Takeover   bool
	}
	LoadOptions struct {
		ProjectDir              string
		ProjectName             string
		OverrideConfigFiles     []string
		Profiles                []string
		EnvFile                 string
		Policy                  ResourcePolicy
		ResourceIdentifierLabel string
		ResourceIdentifier      string
	}

	Composer interface {
		ProjectName() string
		Up(ctx context.Context, service string, opts UpOptions) (*Service, error)
		RemoveCreated(ctx context.Context, opts RemoveOptions) error
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
		Scale         int
		ScalingPolicy ScalingPolicy
	}

	RemoveOptions struct {
		RemoveAnonymousVolumes bool
	}

	Service struct {
		Name         string
		ContainerIDs []string
		Env          map[string]string
	}
)

type ScalingPolicy int

const (
	ScalingPolicyScaleOut ScalingPolicy = iota
	ScalingPolicyConsistent
)
