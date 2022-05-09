package confort

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Group struct {
	cli       *client.Client
	namespace string
	network   string
}

func NewGroup(tb testing.TB, network string, opts ...client.Opt) *Group {
	tb.Helper()

	ctx := context.Background()
	namespace := os.Getenv("CFT_NAMESPACE")
	if network == "" {
		network = os.Getenv("CFT_NETWORK")
	} else if namespace != "" && network != "" {
		network = namespace + "/" + network
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		tb.Fatal(err)
	}

	if network != "" {
		// ネットワークの存在を確認、なければ作成
		list, err := cli.NetworkList(ctx, types.NetworkListOptions{})
		if err != nil {
			tb.Fatal(err)
		}
		var found bool
		for _, n := range list {
			if n.Name == network {
				found = true
				break
			}
		}
		if !found {
			_, err = cli.NetworkCreate(ctx, network, types.NetworkCreate{
				Driver: "bridge",
			})
			if err != nil {
				tb.Fatal(err)
			}
		}
	}

	return &Group{
		cli:       cli,
		namespace: namespace,
		network:   network,
	}
}

type Container struct {
	container.Config
	name    string
	WaitFor wait.Strategy
}

// TODO: delay run option

func (l *Group) Run(tb testing.TB, name string, c *Container) map[string]string {
	tb.Helper()
	ctx := context.Background()

	if l.namespace != "" {
		name = l.namespace + "/" + name
	}

	hostConfig := container.HostConfig{ // TODO: option
		Binds:           nil,
		ContainerIDFile: "",
		LogConfig:       container.LogConfig{},
		NetworkMode:     "",
		PortBindings:    nil,
		RestartPolicy:   container.RestartPolicy{},
		AutoRemove:      false,
		VolumeDriver:    "",
		VolumesFrom:     nil,
		CapAdd:          nil,
		CapDrop:         nil,
		CgroupnsMode:    "",
		DNS:             nil,
		DNSOptions:      nil,
		DNSSearch:       nil,
		ExtraHosts:      nil,
		GroupAdd:        nil,
		IpcMode:         "",
		Cgroup:          "",
		Links:           nil,
		OomScoreAdj:     0,
		PidMode:         "",
		Privileged:      false,
		PublishAllPorts: false,
		ReadonlyRootfs:  false,
		SecurityOpt:     nil,
		StorageOpt:      nil,
		Tmpfs:           nil,
		UTSMode:         "",
		UsernsMode:      "",
		ShmSize:         0,
		Sysctls:         nil,
		Runtime:         "",
		ConsoleSize:     [2]uint{},
		Isolation:       "",
		Resources:       container.Resources{},
		Mounts:          nil,
		MaskedPaths:     nil,
		ReadonlyPaths:   nil,
		Init:            nil,
	}
	networkConfig := network.NetworkingConfig{} // TODO: option

	l.cli.ContainerCreate(ctx, &c.Config, &hostConfig, &networkConfig, nil, name)
	return nil
}

func (l *Group) BuildAndRun(tb testing.TB, dockerfile string, skip bool, name string, c *Container) map[string]string {
	tb.Helper()
	ctx := context.Background()

	// 指定の名前のイメージが既に存在するかどうかの確認
	var found bool
	summaries, err := l.cli.ImageList(ctx, types.ImageListOptions{
		All: true,
	})
	if err != nil {
		tb.Fatal(err)
	}
LOOP:
	for _, s := range summaries {
		for _, t := range s.RepoTags {
			if t == c.Image {
				found = true
				break LOOP
			}
		}
	}

	if !skip || !found {
		f, err := os.CreateTemp("", "Dockerfile.*")
		if err != nil {
			tb.Fatal(err)
		}
		dockerfileName := f.Name()
		defer func() {
			_ = f.Close()
			_ = os.Remove(dockerfileName)
		}()

		_, err = f.WriteString(dockerfile)
		if err != nil {
			tb.Fatal(err)
		}

		archived, err := archive(dockerfileName, dockerfile)
		if err != nil {
			tb.Fatal(err)
		}

		resp, err := l.cli.ImageBuild(ctx, archived, types.ImageBuildOptions{
			Dockerfile: dockerfileName,
			Tags:       []string{c.Image},
			Remove:     true,
		})
		if err != nil {
			tb.Fatal(err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		var buf strings.Builder
		dec := json.NewDecoder(resp.Body)
		for {
			v := map[string]interface{}{}
			err = dec.Decode(&v)
			if err == io.EOF {
				break
			} else if err != nil {
				tb.Fatal(err)
			}
			msg, ok := v["stream"].(string)
			if ok {
				buf.WriteString(msg)
			}
			errorMsg, ok := v["error"]
			if ok {
				tb.Log(buf.String())
				tb.Fatal(errorMsg)
			}
		}
	} else {
		tb.Logf("image %q already exists", c.Image)
	}

	return l.Run(tb, name, c)
}

func archive(dockerfileName, dockerfile string) (io.Reader, error) {
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	err := tw.WriteHeader(&tar.Header{
		Name: dockerfileName,
		Size: int64(len(dockerfile)),
	})
	if err != nil {
		return nil, err
	}
	_, err = tw.Write([]byte(dockerfile))
	if err != nil {
		return nil, err
	}
	err = tw.Close()
	if err != nil {
		return nil, err
	}

	return buf, nil
}
