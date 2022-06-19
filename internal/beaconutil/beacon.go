package beaconutil

import (
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-units"
)

func ConvertBuildOptionsFromProto(opts *beacon.BuildOptions) types.ImageBuildOptions {
	ulimits := make([]*units.Ulimit, 0, len(opts.Ulimits))
	for _, ul := range opts.Ulimits {
		ulimits = append(ulimits, &units.Ulimit{
			Name: ul.Name,
			Hard: ul.Hard,
			Soft: ul.Soft,
		})
	}
	buildArgs := map[string]*string{}
	for _, arg := range opts.BuildArgs {
		buildArgs[arg.Name] = arg.Value
	}
	authConfigs := map[string]types.AuthConfig{}
	for k, v := range opts.AuthConfigs {
		authConfigs[k] = types.AuthConfig{
			Username:      v.UserName,
			Password:      v.Password,
			Auth:          v.Auth,
			Email:         v.Email,
			ServerAddress: v.ServerAddress,
			IdentityToken: v.IdentityToken,
			RegistryToken: v.RegistryToken,
		}
	}

	return types.ImageBuildOptions{
		Tags:           opts.Tags,
		SuppressOutput: opts.SuppressOutput,
		RemoteContext:  opts.RemoteContext,
		NoCache:        opts.NoCache,
		Remove:         opts.Remove,
		ForceRemove:    opts.ForceRemove,
		PullParent:     opts.PullParent,
		Isolation:      container.Isolation(opts.Isolation),
		CPUSetCPUs:     opts.CpuSetCpus,
		CPUSetMems:     opts.CpuSetMems,
		CPUShares:      opts.CpuShares,
		CPUQuota:       opts.CpuQuota,
		CPUPeriod:      opts.CpuPeriod,
		Memory:         opts.Memory,
		MemorySwap:     opts.MemorySwap,
		CgroupParent:   opts.CgroupParent,
		NetworkMode:    opts.NetworkMode,
		ShmSize:        opts.ShmSize,
		Dockerfile:     opts.Dockerfile,
		Ulimits:        ulimits,
		BuildArgs:      buildArgs,
		AuthConfigs:    authConfigs,
		// Context:        nil,
		Labels:      opts.Labels,
		Squash:      opts.Squash,
		CacheFrom:   opts.CacheFrom,
		SecurityOpt: opts.SecurityOpt,
		ExtraHosts:  opts.ExtraHosts,
		Target:      opts.Target,
		SessionID:   opts.SessionId,
		Platform:    opts.Platform,
		// Version:        "",
		// BuildID:        "",
		// Outputs:        nil,
	}
}

func ConvertBuildOptionsToProto(opts types.ImageBuildOptions) *beacon.BuildOptions {
	ulimits := make([]*beacon.Ulimit, 0, len(opts.Ulimits))
	for _, ul := range opts.Ulimits {
		ulimits = append(ulimits, &beacon.Ulimit{
			Name: ul.Name,
			Hard: ul.Hard,
			Soft: ul.Soft,
		})
	}
	buildArgs := make([]*beacon.BuildArg, 0, len(opts.BuildArgs))
	for name, value := range opts.BuildArgs {
		buildArgs = append(buildArgs, &beacon.BuildArg{
			Name:  name,
			Value: value,
		})
	}
	authConfigs := map[string]*beacon.AuthConfig{}
	for k, v := range opts.AuthConfigs {
		authConfigs[k] = &beacon.AuthConfig{
			UserName:      v.Username,
			Password:      v.Password,
			Auth:          v.Auth,
			Email:         v.Email,
			ServerAddress: v.ServerAddress,
			IdentityToken: v.IdentityToken,
			RegistryToken: v.RegistryToken,
		}
	}

	return &beacon.BuildOptions{
		Tags:           opts.Tags,
		SuppressOutput: opts.SuppressOutput,
		RemoteContext:  opts.RemoteContext,
		NoCache:        opts.NoCache,
		Remove:         opts.Remove,
		ForceRemove:    opts.ForceRemove,
		PullParent:     opts.PullParent,
		Isolation:      string(opts.Isolation),
		CpuSetCpus:     opts.CPUSetCPUs,
		CpuSetMems:     opts.CPUSetMems,
		CpuShares:      opts.CPUShares,
		CpuQuota:       opts.CPUQuota,
		CpuPeriod:      opts.CPUPeriod,
		Memory:         opts.Memory,
		MemorySwap:     opts.MemorySwap,
		CgroupParent:   opts.CgroupParent,
		NetworkMode:    opts.NetworkMode,
		ShmSize:        opts.ShmSize,
		Dockerfile:     opts.Dockerfile,
		Ulimits:        ulimits,
		BuildArgs:      buildArgs,
		AuthConfigs:    authConfigs,
		Labels:         opts.Labels,
		Squash:         opts.Squash,
		CacheFrom:      opts.CacheFrom,
		SecurityOpt:    opts.SecurityOpt,
		ExtraHosts:     opts.ExtraHosts,
		Target:         opts.Target,
		SessionId:      opts.SessionID,
		Platform:       opts.Platform,
	}
}

func ConvertPullOptionsFromProto(v *beacon.PullOptions) *types.ImagePullOptions {
	if v == nil {
		return nil
	}
	return &types.ImagePullOptions{
		All:          v.All,
		RegistryAuth: v.RegistryAuth,
		Platform:     v.Platform,
	}
}

func ConvertPullOptionsToProto(v *types.ImagePullOptions) *beacon.PullOptions {
	if v == nil {
		return nil
	}
	return &beacon.PullOptions{
		All:          v.All,
		RegistryAuth: v.RegistryAuth,
		Platform:     v.Platform,
	}
}
