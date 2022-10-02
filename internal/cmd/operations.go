package cmd

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"github.com/daichitakahashi/confort/internal/beacon/server"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/lestrrat-go/backoff/v2"
	"go.uber.org/multierr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Operation interface {
	StartBeaconServer(ctx context.Context) (string, <-chan struct{}, error)
	StopBeaconServer(ctx context.Context, addr string) error
	CleanupResources(ctx context.Context, label, value string) error
	ExecuteTest(ctx context.Context, goCmd string, args []string, environments []string) error
}

type operation struct {
	srv *grpc.Server
	ln  net.Listener
}

func NewOperation() Operation {
	srv := grpc.NewServer(
		grpc.ConnectionTimeout(time.Minute * 5), // TODO: configure
	)

	op := &operation{
		srv: srv,
	}
	server.Register(srv, func() error {
		go func() {
			<-time.After(time.Millisecond * 500)
			op.srv.Stop()
			_ = op.ln.Close()
		}()
		return nil
	})
	return op
}

func (o *operation) StartBeaconServer(ctx context.Context) (_ string, _ <-chan struct{}, err error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", nil, err
	}
	o.ln = ln

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := o.srv.Serve(ln)
		if err != nil {
			log.Fatal(err)
		}
	}()
	defer func() {
		if err != nil { // failed
			o.srv.Stop()
			_ = ln.Close()
		}
	}()

	conn, err := grpc.DialContext(ctx, ln.Addr().String(), grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		return "", nil, err
	}
	defer func() {
		_ = conn.Close()
	}()
	cli := health.NewHealthClient(conn)

	b := backoff.Constant(
		backoff.WithInterval(200*time.Millisecond),
		backoff.WithMaxRetries(150),
	).Start(ctx)
	for backoff.Continue(b) {
		resp, err := cli.Check(ctx, &health.HealthCheckRequest{
			Service: "beacon",
		})
		if err != nil {
			continue
		}
		if resp.Status == health.HealthCheckResponse_SERVING {
			return ln.Addr().String(), done, nil
		}
	}

	return "", nil, errors.New("cannot obtain beacon endpoint")
}

func (o *operation) StopBeaconServer(ctx context.Context, addr string) error {
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	cli := proto.NewBeaconServiceClient(conn)
	_, err = cli.Interrupt(ctx, &emptypb.Empty{})
	return err
}

func (o *operation) CleanupResources(ctx context.Context, label, value string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	cli.NegotiateAPIVersion(ctx)

	f := filters.NewArgs(
		filters.Arg("label", label+"="+value),
	)

	var errs []error

	// remove container
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		errs = append(errs, err)
	}
	for _, c := range containers {
		err := cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		})
		if err != nil {
			errs = append(errs, err)
		}
	}

	// remove network
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{
		Filters: f,
	})
	if err != nil {
		errs = append(errs, err)
	}
	for _, n := range networks {
		err := cli.NetworkRemove(ctx, n.ID)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return multierr.Combine(errs...)
}

func (o *operation) ExecuteTest(ctx context.Context, goCmd string, args, environments []string) error {
	cmd := exec.CommandContext(ctx, goCmd, append([]string{"test"}, args...)...)
	cmd.Env = environments
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var _ Operation = (*operation)(nil)
