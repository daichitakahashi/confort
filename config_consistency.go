package confort

import (
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/goccy/go-reflect"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/multierr"
)

func checkConfigConsistency(
	container1, container2 *container.Config,
	host1, host2 *container.HostConfig,
	network1, network2 map[string]*network.EndpointSettings,
) (err error) {
	if err = checkContainerConfigConsistency(container2, container1); err != nil {
		return fmt.Errorf("inconsistent container config\n %w", err)
	}
	if err = checkHostConfigConsistency(host2, host1); err != nil {
		return fmt.Errorf("inconsistent host config\n%w", err)
	}
	if err = checkEndpointSettingsConsistency(network2, network1); err != nil {
		return fmt.Errorf("inconsistent network config\n%w", err)
	}
	return nil
}

func checkContainerConfigConsistency(expected, target *container.Config) error {
	return multierr.Combine(
		stringSubset("Hostname", expected.Hostname, target.Hostname),
		stringSubset("Hostname", expected.Hostname, target.Hostname),
		stringSubset("Domainname", expected.Domainname, target.Domainname),
		stringSubset("User", expected.User, target.User),
		// AttachStdin
		// AttachStdout
		// AttachStderr
		mapSubset("ExposedPorts", expected.ExposedPorts, target.ExposedPorts),
		// Tty
		// OpenStdin
		// StdinOnce
		sliceSubset("Env", expected.Env, target.Env),
		sequentialSubset("Cmd", expected.Cmd, target.Cmd),
		pointerSubset("Healthcheck", expected.Healthcheck, target.Healthcheck),
		equals("ArgsEscaped", expected.ArgsEscaped, target.ArgsEscaped),
		equals("Image", expected.Image, target.Image),
		mapSubset("Volumes", expected.Volumes, target.Volumes),
		equals("WorkingDir", expected.WorkingDir, target.WorkingDir),
		sequentialSubset("Entrypoint", expected.Entrypoint, target.Entrypoint),
		// NetworkDisabled
		stringSubset("MacAddress", expected.MacAddress, target.MacAddress),
		// OnBuild
		mapSubset("Labels", expected.Labels, target.Labels),
		stringSubset("StopSignal", expected.StopSignal, target.StopSignal),
		pointerSubset("StopTimeout", expected.StopTimeout, target.StopTimeout),
		sequentialSubset("Shell", expected.Shell, target.Shell),
	)
}

func checkHostConfigConsistency(expected, target *container.HostConfig) error {
	return multierr.Combine(
		sliceSubset("Binds", expected.Binds, target.Binds),
		// ContainerIDFile string
		// LogConfig
		stringSubset("NetworkMode", string(expected.NetworkMode), string(target.NetworkMode)),
		// TODO: ("PortBindings", expected.PortBindings, target.PortBindings),
		stringSubset("RestartPolicy", expected.RestartPolicy.Name, target.RestartPolicy.Name),
		// AutoRemove
		stringSubset("VolumeDriver", expected.VolumeDriver, target.VolumeDriver),
		sliceSubset("VolumesFrom", expected.VolumesFrom, target.VolumesFrom),

		// Applicable to UNIX platforms
		sliceSubset("CapAdd", expected.CapAdd, target.CapAdd),
		sliceSubset("CapDrop", expected.CapDrop, target.CapDrop),
		stringSubset("CgroupsMode", string(expected.CgroupnsMode), string(target.CgroupnsMode)),
		sliceSubset("DNS", expected.DNS, target.DNS),
		sliceSubset("DNSOptions", expected.DNSOptions, target.DNSOptions),
		sliceSubset("DNSSearch", expected.DNSSearch, target.DNSSearch),
		sliceSubset("ExtraHosts", expected.ExtraHosts, target.ExtraHosts),
		sliceSubset("GroupAdd", expected.GroupAdd, target.GroupAdd),
		stringSubset("IpcMode", string(expected.IpcMode), string(target.IpcMode)),
		stringSubset("Cgroup", string(expected.Cgroup), string(target.Cgroup)),
		sliceSubset("Links", expected.Links, target.Links),
		// OomScoreAdj
		stringSubset("PidMode", string(expected.PidMode), string(target.PidMode)),
		equals("Privileged", expected.Privileged, target.Privileged),
		equals("PublishAllPorts", expected.PublishAllPorts, target.PublishAllPorts),
		equals("ReadonlyRootfs", expected.ReadonlyRootfs, target.ReadonlyRootfs),
		sliceSubset("SecurityOpt", expected.SecurityOpt, target.SecurityOpt),
		mapSubset("StorageOpt", expected.StorageOpt, target.StorageOpt),
		mapSubset("Tmpfs", expected.Tmpfs, target.Tmpfs),
		// UTSMode
		// UsernsMode
		// ShmSize
		// Sysctls
		// Runtime

		// Applicable to Windows
		// ConsoleSize
		// Isolation

		// Contains container's resources (cgroups, ulimits)
		// Resources

		// Mounts specs used by the container
		// TODO: ("Mounts", expected.Mounts, target.Mounts),

		// MaskedPaths is the list of paths to be masked inside the container (this overrides the default set of paths)
		// MaskedPaths

		// ReadonlyPaths is the list of paths to be set as read-only inside the container (this overrides the default set of paths)
		// ReadonlyPaths

		// Run a custom init inside the container, if null, use the daemon's configured settings
		// Init
	)
}

func checkEndpointSettingsConsistency(expected, target map[string]*network.EndpointSettings) (err error) {
	for networkName, settings := range target {
		var e error
		if expectedSettings, ok := expected[networkName]; !ok {
			// e = fmt.Errorf("%s: %s", networkName, cmp.Diff(expectedSettings, settings))
		} else {
			e = multierr.Combine(
				pointerSubset("IPAMConfig", expectedSettings.IPAMConfig, settings.IPAMConfig),
				sliceSubset("Links", expectedSettings.Links, settings.Links),
				sliceSubset("Aliases", expectedSettings.Aliases, settings.Aliases),
				equals("NetworkID", expectedSettings.NetworkID, settings.NetworkID),
				// EndpointID
				stringSubset("Gateway", expectedSettings.Gateway, settings.Gateway),
				stringSubset("IPAddress", expectedSettings.IPAddress, settings.IPAddress),
				// equals("IPPrefixLen", expectedSettings.IPPrefixLen, settings.IPPrefixLen),
				stringSubset("IPv6Gateway", expectedSettings.IPv6Gateway, settings.IPv6Gateway),
				stringSubset("GlobalIPv6Address", expectedSettings.GlobalIPv6Address, settings.GlobalIPv6Address),
				// equals("GlobalIPv6PrefixLen", expectedSettings.GlobalIPv6PrefixLen, settings.GlobalIPv6PrefixLen),
				stringSubset("MacAddress", expectedSettings.MacAddress, settings.MacAddress),
				mapSubset("DriverOpts", expectedSettings.DriverOpts, settings.DriverOpts),
			)
		}
		if e != nil {
			err = multierr.Append(err, fmt.Errorf("%s: %w", networkName, e))
		}
	}
	return err
}

func stringSubset(name, expected, target string) (err error) {
	if target == "" || target == expected {
		return nil
	}
	return diffError(name, expected, target)
}

func sliceSubset[T comparable](name string, expected, target []T) (err error) {
	if len(target) == 0 {
		return nil
	} else if len(expected) == 0 {
		return diffError(name, expected, target)
	}
	exp := make(map[T]bool)
	for _, t := range expected {
		exp[t] = true
	}
	for _, t := range target {
		if !exp[t] {
			return diffError(name, expected, target)
		}
	}
	return nil
}

func sequentialSubset[T comparable](name string, expected, target []T) (err error) {
	if len(target) == 0 {
		return nil
	} else if len(expected) == 0 || !reflect.DeepEqual(expected, target) {
		return diffError(name, expected, target)
	}
	return nil
}

func mapSubset[K, V comparable](name string, expected, target map[K]V) error {
	if len(target) == 0 {
		return nil
	} else if len(expected) == 0 {
		return diffError(name, expected, target)
	}
	for k, v := range target {
		e, ok := expected[k]
		if !ok || e != v {
			return diffError(name, expected, target)
		}
	}
	return nil
}

func pointerSubset[T any](name string, expected, target *T) (err error) {
	if target == nil {
		return nil
	} else if expected == nil || !reflect.DeepEqual(expected, target) {
		return diffError(name, expected, target)
	}
	return nil
}

func equals(name string, expected, target any) error {
	diff := cmp.Diff(expected, target)
	if diff != "" {
		return fmt.Errorf("%s\n%s", name, diff)
	}
	return nil
}

func diffError(msg string, expected, target any) error {
	diff := cmp.Diff(expected, target)
	return fmt.Errorf("%s\n%s", msg, diff)
}
