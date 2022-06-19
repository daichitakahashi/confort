package confort

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/daichitakahashi/oncewait"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
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
//  - export CFT_BEACON_ENDPOINT=`go run github.com/daichitakahashi/confort/cmd/confort run -p 9999`

// client of beacon container
type beaconBackend struct {
	cli beacon.BeaconServiceClient
}

func (b *beaconBackend) Namespace(ctx context.Context, namespace string) (Namespace, error) {
	resp, err := b.cli.Register(ctx, &beacon.RegisterRequest{
		Namespace: namespace,
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

func (b *beaconBackend) BuildImage(ctx context.Context, buildContext io.Reader, buildOptions types.ImageBuildOptions, force bool, buildOut io.Writer) error {
	if len(buildOptions.Tags) == 0 {
		return errors.New("image tag not specified")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := b.cli.BuildImage(ctx)
	if err != nil {
		return err
	}

	err = stream.Send(&beacon.BuildImageRequest{
		Build: &beacon.BuildImageRequest_BuildInfo{
			BuildInfo: &beacon.BuildInfo{
				BuildOptions: beaconutil.ConvertBuildOptionsToProto(buildOptions),
				Force:        force,
			},
		},
	})
	if err != nil {
		return err
	}

	r := bufio.NewReaderSize(buildContext, 4*1024)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			switch vt := resp.GetProcessing().(type) {
			case *beacon.BuildImageResponse_Building:
				_, err = buildOut.Write(vt.Building.GetMessage())
				if err != nil {
					return err
				}
			case *beacon.BuildImageResponse_Built:
				return nil
			}
		}
	})
	eg.Go(func() error {
		buf := make([]byte, 4*1024)
		for {
			select {
			case <-ctx.Done():
				return multierr.Append(ctx.Err(), stream.CloseSend())
			default:
			}
			n, err := r.Read(buf)
			if n > 0 {
				err = stream.Send(&beacon.BuildImageRequest{
					Build: &beacon.BuildImageRequest_Context{
						Context: buf[:n],
					},
				})
				if err != nil {
					return multierr.Append(err, stream.CloseSend())
				}
			}
			if err == io.EOF {
				return stream.CloseSend()
			}
			if err != nil {
				return multierr.Append(err, stream.CloseSend())
			}
		}
	})
	return eg.Wait()
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
	pull := beaconutil.ConvertPullOptionsToProto(pullOptions)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := b.cli.CreateContainer(ctx, &beacon.CreateContainerRequest{
		ClientId:               b.clientID,
		Name:                   name,
		ContainerConfig:        cc,
		HostConfig:             hc,
		NetworkingConfig:       nc,
		CheckConfigConsistency: configConsistency,
		PullOptions:            pull,
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
			_, err = pullOut.Write(vt.Pulling.GetMessage())
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
		ClientId: b.clientID,
	})
	return err
}

var _ Namespace = (*beaconNamespace)(nil)
