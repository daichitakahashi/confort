package confort

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/daichitakahashi/oncewait"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	beaconConn *grpc.ClientConn
	connOnce   = oncewait.New()
)

func ConnectBeacon(tb testing.TB, ctx context.Context) (*grpc.ClientConn, bool) {
	target := os.Getenv("CFT_BEACON_ENDPOINT")
	if target == "" {
		return nil, false
	}

	connOnce.Do(func() {
		var err error
		beaconConn, err = grpc.DialContext(ctx, target,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			tb.Fatalf("ConnectBeacon: %s", err)
		}
	})
	return beaconConn, true
}

// TODO:
//  - export CFT_BEACON_ENDPOINT=`go run github.com/daichitakahashi/confort/cmd/confort -p 9999 -namespace hogehoge*`

// client of beacon container
type beaconBackend struct {
	cli    beacon.BeaconServiceClient
	policy beacon.ResourcePolicy
}

func (b *beaconBackend) Namespace(ctx context.Context, namespace string) (Namespace, error) {
	resp, err := b.cli.Register(ctx, &beacon.RegisterRequest{
		Namespace:      namespace,
		ResourcePolicy: b.policy,
	})
	if err != nil {
		return nil, err
	}

	var nw types.NetworkResource
	err = json.Unmarshal(resp.GetNetworkResource(), &nw)
	if err != nil {
		return nil, err
	}

	return &beaconNamespace{
		clientID:  resp.GetClientId(),
		namespace: namespace,
		cli:       b.cli,
		network:   &nw,
	}, nil
}

func (b *beaconBackend) BuildImage(ctx context.Context, contextDir string, buildOptions types.ImageBuildOptions, force bool, buildOut io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	options, err := json.Marshal(buildOptions)
	if err != nil {
		return err
	}
	stream, err := b.cli.BuildImage(ctx, &beacon.BuildImageRequest{
		ContextDir:   contextDir,
		BuildOptions: options,
		Force:        force,
	})
	if err != nil {
		return err
	}

streaming:
	for {
		resp, err := stream.Recv()
		if err != nil {
			return err
		}
		switch vt := resp.GetProcessing().(type) {
		case *beacon.BuildImageResponse_Building:
			_, err = io.WriteString(buildOut, vt.Building.GetMessage())
			if err != nil {
				return err
			}
		case *beacon.BuildImageResponse_Built:
			break streaming
		}
	}
	return nil
}

var _ Backend = (*beaconBackend)(nil)

type beaconNamespace struct {
	namespace string
	clientID  string
	cli       beacon.BeaconServiceClient
	network   *types.NetworkResource
}

func (b *beaconNamespace) Namespace() string {
	return b.namespace
}

func (b *beaconNamespace) Network() *types.NetworkResource {
	return b.network
}

func (b *beaconNamespace) CreateContainer(ctx context.Context, name string, container *container.Config, host *container.HostConfig, network *network.NetworkingConfig, configConsistency bool, pullOptions *types.ImagePullOptions, pullOut io.Writer) (string, error) {
	cc, err := json.Marshal(container)
	if err != nil {
		return "", err
	}
	hc, err := json.Marshal(host)
	if err != nil {
		return "", err
	}
	nc, err := json.Marshal(network)
	if err != nil {
		return "", err
	}
	var pull []byte
	if pullOptions != nil {
		pull, err = json.Marshal(pullOptions)
		if err != nil {
			return "", err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := b.cli.CreateContainer(ctx, &beacon.CreateContainerRequest{
		ClientId:               b.clientID,
		Name:                   name,
		ContainerConfig:        cc,
		HostConfig:             hc,
		NetworkingConfig:       nc,
		CheckConfigConsistency: configConsistency,
		PullConfig:             pull,
	})
	if err != nil {
		return "", err
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			return "", err
		}
		switch vt := resp.GetProcessing().(type) {
		case *beacon.CreateContainerResponse_Pulling:
			_, err = io.WriteString(pullOut, vt.Pulling.GetMessage())
			if err != nil {
				return "", err
			}
		case *beacon.CreateContainerResponse_Created:
			return vt.Created.GetContainerId(), nil
		}
	}
}

func (b *beaconNamespace) StartContainer(ctx context.Context, name string, exclusive bool) (nat.PortMap, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := b.cli.AcquireContainerEndpoint(ctx, &beacon.AcquireContainerEndpointRequest{
		ClientId:      b.clientID,
		ContainerName: name,
		Exclusive:     exclusive,
	})
	if err != nil {
		return nil, err
	}

	// wait for acquire lock
	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	portMap := nat.PortMap{}
	for port, endpoints := range resp.GetEndpoints() {
		bindings := make([]nat.PortBinding, len(endpoints.Bindings))
		for _, binding := range endpoints.Bindings {
			bindings = append(bindings, nat.PortBinding{
				HostIP:   binding.GetHostIp(),
				HostPort: binding.GetHostPort(),
			})
		}
		portMap[nat.Port(port)] = bindings
	}
	return portMap, nil
}

func (b *beaconNamespace) ReleaseContainer(ctx context.Context, name string, _ bool) error {
	_, err := b.cli.ReleaseContainer(ctx, &beacon.ReleaseContainerRequest{
		ClientId:      b.clientID,
		ContainerName: name,
	})
	return err
}

func (b *beaconNamespace) Release(ctx context.Context) error {
	_, err := b.cli.Deregister(ctx, &beacon.DeregisterRequest{
		ClientId:  b.clientID,
		Namespace: b.namespace,
	})
	return err
}

var _ Namespace = (*beaconNamespace)(nil)
