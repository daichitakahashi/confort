package confort

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Group struct {
	cli       *client.Client
	namespace string
	network   *types.NetworkResource

	m         sync.Mutex
	terminate []TerminateFunc
	endpoints map[string]map[nat.Port]string
}

type TerminateFunc func()

func NewGroup(ctx context.Context, tb testing.TB, namespace, network string, opts ...client.Opt) (*Group, TerminateFunc) {
	tb.Helper()

	if namespace == "" {
		namespace = os.Getenv("CFT_NAMESPACE")
	}
	if network == "" {
		network = os.Getenv("CFT_NETWORK")
	} else if namespace != "" && network != "" {
		network = namespace + "-" + network
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		tb.Fatal(err)
	}
	cli.NegotiateAPIVersion(ctx)

	var nw *types.NetworkResource
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
			// TODO: **remove network**
			created, err := cli.NetworkCreate(ctx, network, types.NetworkCreate{
				Driver: "bridge",
			})
			if err != nil {
				tb.Fatal(err)
			}
			n, err := cli.NetworkInspect(ctx, created.ID, types.NetworkInspectOptions{
				Verbose: true,
			})
			if err != nil {
				tb.Fatal(err)
			}
			nw = &n
		}
	}

	g := &Group{
		cli:       cli,
		namespace: namespace,
		network:   nw,
		endpoints: map[string]map[nat.Port]string{},
	}
	term := func() {
		ctx := context.Background()

		last := len(g.terminate) - 1
		for i := range g.terminate {
			g.terminate[last-i]()
		}

		if g.network != nil {
			err := g.cli.NetworkRemove(ctx, g.network.ID)
			if err != nil {
				tb.Logf("error occurred in remove network %q: %s", network, err)
			}
		}
	}

	return g, term
}

type Container struct {
	Image        string
	Env          []string
	Cmd          []string
	Entrypoint   []string
	ExposedPorts []string
	WaitFor      wait.Strategy
}

type RunOption struct {
	// Lazy          bool // TODO: Group.LazyRun(ctx, &confort.M{}, name, c)???
	ContainerConfig func(config *container.Config)
	HostConfig      func(config *container.HostConfig)
	NetworkConfig   func(config *network.NetworkingConfig)
	PullOptions     *types.ImagePullOptions
}

func (g *Group) Run(ctx context.Context, tb testing.TB, name string, c *Container, opt RunOption) map[nat.Port]string {
	tb.Helper()

	if g.namespace != "" {
		name = g.namespace + "-" + name
	}

	g.m.Lock()
	defer g.m.Unlock()

	// find cached endpoints
	endpoints, ok := g.endpoints[name]
	if ok {
		return endpoints
	}

	// find existing container
	existing, err := g.existingContainer(ctx, c.Image, name)
	if err != nil {
		tb.Fatal(err)
	} else if existing != nil {
		endpoints = map[nat.Port]string{}
		for _, p := range existing.Ports {
			if p.IP != "" {
				np, err := nat.NewPort(p.Type, fmt.Sprint(p.PrivatePort))
				if err != nil {
					tb.Fatal(err)
				}
				endpoints[np] = p.IP + ":" + fmt.Sprint(p.PublicPort)
			}
		}
		g.endpoints[name] = endpoints
		return endpoints
	}

	if opt.PullOptions != nil {
		rc, err := g.cli.ImagePull(ctx, c.Image, *opt.PullOptions)
		if err != nil {
			tb.Fatalf("pull: %s", err)
		}
		_, err = io.ReadAll(rc)
		if err != nil {
			tb.Fatalf("pull: %s", err)
		}
		err = rc.Close()
		if err != nil {
			tb.Fatalf("pull: %s", err)
		}
	}

	portSet, portBindings, err := nat.ParsePortSpecs(c.ExposedPorts)
	if err != nil {
		tb.Fatal(err)
	}

	cc := &container.Config{
		Image:        c.Image,
		ExposedPorts: portSet,
		Env:          c.Env,
		Cmd:          c.Cmd,
		Entrypoint:   c.Entrypoint,
	}
	if opt.ContainerConfig != nil {
		opt.ContainerConfig(cc)
	}
	hc := &container.HostConfig{
		PortBindings: portBindings,
		AutoRemove:   true,
	}
	if opt.HostConfig != nil {
		opt.HostConfig(hc)
	}
	nc := &network.NetworkingConfig{}
	if g.network != nil {
		nc.EndpointsConfig = map[string]*network.EndpointSettings{
			g.network.ID: {
				NetworkID: g.network.ID,
			},
		}
	}
	if opt.NetworkConfig != nil {
		opt.NetworkConfig(nc)
	}

	created, err := g.cli.ContainerCreate(ctx, cc, hc, nc, nil, name)
	if err != nil {
		tb.Fatal(err)
	}

	err = g.cli.ContainerStart(ctx, created.ID, types.ContainerStartOptions{})
	if err != nil {
		tb.Fatal(err)
	}

	var success bool
	term := func() {
		err := g.cli.ContainerStop(context.Background(), created.ID, nil)
		if err != nil {
			tb.Log(err)
		}
	}
	defer func() {
		if !success {
			term()
		}
	}()

	i, err := g.cli.ContainerInspect(ctx, created.ID)
	if err != nil {
		tb.Fatal(err)
	}
	endpoints = map[nat.Port]string{}
	for p, bindings := range i.NetworkSettings.Ports {
		if len(bindings) == 0 {
			continue
		}
		b := bindings[0]
		if b.HostPort == "" {
			continue
		}
		endpoints[p] = b.HostIP + ":" + b.HostPort
	}

	g.terminate = append(g.terminate, term)
	g.endpoints[name] = endpoints
	success = true
	return endpoints
}

func (g *Group) existingContainer(ctx context.Context, image, name string) (*types.Container, error) {
	name = "/" + name

	containers, err := g.cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if name == n {
				if c.Image == image {
					return &c, nil
				}
				return nil, fmt.Errorf("container name %q already exists but image is not %q(%q)", name, image, c.Image)
			}
		}
	}
	return nil, nil
}

func (g *Group) BuildAndRun(ctx context.Context, tb testing.TB, dockerfile string, skip bool, name string, c *Container, opt RunOption) map[nat.Port]string {
	tb.Helper()

	// 指定の名前のイメージが既に存在するかどうかの確認
	var found bool
	summaries, err := g.cli.ImageList(ctx, types.ImageListOptions{
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

		resp, err := g.cli.ImageBuild(ctx, archived, types.ImageBuildOptions{
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

	opt.PullOptions = nil
	return g.Run(ctx, tb, name, c, opt)
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
