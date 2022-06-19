package server

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type beaconServer struct {
	beacon.UnimplementedBeaconServiceServer

	be               confort.Backend
	m                sync.RWMutex
	namespaces       map[string]*namespace
	clientsNamespace map[string]*namespace
}

type acquireType bool

type namespace struct {
	ns      confort.Namespace
	m       sync.Mutex
	clients map[string]map[string]acquireType
}

var unique = confort.NewUnique(func() (string, error) {
	return uuid.New().String(), nil
})

// Register client and create namespace
func (b *beaconServer) Register(ctx context.Context, req *beacon.RegisterRequest) (*beacon.RegisterResponse, error) {
	b.m.Lock()
	defer b.m.Unlock()

	ns, ok := b.namespaces[req.GetNamespace()]
	if !ok {
		// create new namespace if not created yet
		dockerNamespace, err := b.be.Namespace(ctx, req.GetNamespace())
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		ns = &namespace{
			ns:      dockerNamespace,
			clients: map[string]map[string]acquireType{},
		}
		b.namespaces[req.GetNamespace()] = ns
	}

	// issue unique clientID
	clientID, err := unique.New()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	b.clientsNamespace[clientID] = ns
	ns.clients[clientID] = map[string]acquireType{}

	nr, err := json.Marshal(ns.ns.Network())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &beacon.RegisterResponse{
		ClientId:        clientID,
		NetworkResource: nr,
	}, nil
}

// Deregister client
func (b *beaconServer) Deregister(_ context.Context, req *beacon.DeregisterRequest) (*emptypb.Empty, error) {
	b.m.Lock()
	defer b.m.Unlock()

	clientID := req.GetClientId()
	ns, ok := b.clientsNamespace[clientID]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "clientID not found")
	}
	containers := ns.clients[clientID]
	if len(containers) > 0 {
		return nil, status.Error(codes.InvalidArgument, "acquired containers not released")
	}

	delete(b.clientsNamespace, clientID)
	return &emptypb.Empty{}, nil
}

func (b *beaconServer) BuildImage(stream beacon.BeaconService_BuildImageServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	bi := req.GetBuildInfo()
	if bi == nil {
		return status.Error(codes.InvalidArgument, "first message must contains BuildInfo")
	}
	opts := beaconutil.ConvertBuildOptionsFromProto(bi.BuildOptions)

	pr, pw := io.Pipe()
	go func() {
		for {
			req, err := stream.Recv()
			if err == io.EOF {
				_ = pw.Close()
				return
			}
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			buildContext := req.GetContext()
			if len(buildContext) == 0 {
				continue
			}
			_, err = pw.Write(buildContext)
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}
	}()

	w := writerFunc(func(p []byte) (int, error) {
		err := stream.Send(&beacon.BuildImageResponse{
			Processing: &beacon.BuildImageResponse_Building{
				Building: &beacon.Message{
					Message: p,
				},
			},
		})
		return len(p), err
	})

	return b.be.BuildImage(stream.Context(), pr, opts, bi.Force, w)
}

type writerFunc func(p []byte) (int, error)

func (w writerFunc) Write(p []byte) (int, error) { return w(p) }

var _ io.Writer = (writerFunc)(nil)

func (b *beaconServer) CreateContainer(req *beacon.CreateContainerRequest, stream beacon.BeaconService_CreateContainerServer) error {
	clientID := req.GetClientId()
	b.m.RLock()
	ns, ok := b.clientsNamespace[clientID]
	b.m.RUnlock()
	if !ok {
		return status.Error(codes.InvalidArgument, "clientID not found")
	}

	cc, hc, nc, err := unmarshalConfig(
		req.GetContainerConfig(),
		req.GetHostConfig(),
		req.GetNetworkingConfig(),
	)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	pullOptions := beaconutil.ConvertPullOptionsFromProto(req.GetPullOptions())

	w := writerFunc(func(p []byte) (int, error) {
		err := stream.Send(&beacon.CreateContainerResponse{
			Processing: &beacon.CreateContainerResponse_Pulling{
				Pulling: &beacon.Message{
					Message: p,
				},
			},
		})
		return len(p), err
	})

	containerID, err := ns.ns.CreateContainer(stream.Context(), req.GetName(),
		cc, hc, nc, req.GetCheckConfigConsistency(), pullOptions, w)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	return stream.Send(&beacon.CreateContainerResponse{
		Processing: &beacon.CreateContainerResponse_Created{
			Created: &beacon.CreatedContainer{
				ContainerId: containerID,
			},
		},
	})
}

func unmarshalConfig(cc, hc, nc []byte) (*container.Config, *container.HostConfig, *network.NetworkingConfig, error) {
	var (
		config        container.Config
		hostConfig    container.HostConfig
		networkConfig network.NetworkingConfig
	)
	err := json.Unmarshal(cc, &config)
	if err != nil {
		return nil, nil, nil, err
	}
	err = json.Unmarshal(hc, &hostConfig)
	if err != nil {
		return nil, nil, nil, err
	}
	err = json.Unmarshal(nc, &networkConfig)
	if err != nil {
		return nil, nil, nil, err
	}
	return &config, &hostConfig, &networkConfig, nil
}

func (b *beaconServer) AcquireContainerEndpoint(req *beacon.AcquireContainerEndpointRequest, stream beacon.BeaconService_AcquireContainerEndpointServer) error {
	clientID := req.GetClientId()
	b.m.RLock()
	ns, ok := b.clientsNamespace[clientID]
	b.m.RUnlock()
	if !ok {
		return status.Error(codes.InvalidArgument, "clientID not found")
	}

	containerName := req.GetContainerName()
	exclusive := req.GetExclusive()
	ports, err := ns.ns.StartContainer(stream.Context(), containerName, exclusive)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	endpoints := map[string]*beacon.Endpoints{}
	for port, bindings := range ports {
		endpoint := beacon.Endpoints{
			Bindings: make([]*beacon.PortBinding, 0, len(bindings)),
		}
		for _, binding := range bindings {
			endpoint.Bindings = append(endpoint.Bindings, &beacon.PortBinding{
				HostIp:   binding.HostIP,
				HostPort: binding.HostPort,
			})
		}
		endpoints[string(port)] = &endpoint
	}

	ns.m.Lock()
	containers := ns.clients[clientID]
	containers[containerName] = acquireType(exclusive)
	ns.m.Unlock()

	return stream.Send(&beacon.AcquireContainerEndpointResponse{
		Endpoints: endpoints,
	})
}

func (b *beaconServer) ReleaseContainer(ctx context.Context, req *beacon.ReleaseContainerRequest) (*emptypb.Empty, error) {
	clientID := req.GetClientId()
	b.m.RLock()
	ns, ok := b.clientsNamespace[clientID]
	b.m.RUnlock()
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "clientID not found")
	}

	containerName := req.GetContainerName()
	ns.m.Lock()
	containers := ns.clients[clientID]
	exclusive, ok := containers[containerName]
	ns.m.Unlock()
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "container not found")
	}

	err := ns.ns.ReleaseContainer(ctx, containerName, bool(exclusive))
	if err != nil {
		return nil, status.Error(codes.Unknown, err.Error())
	}
	return &emptypb.Empty{}, nil
}

var _ beacon.BeaconServiceServer = (*beaconServer)(nil)

func (b *beaconServer) Shutdown(ctx context.Context) error {
	b.m.Lock()
	defer b.m.Unlock()

	releaseErr := map[string]string{}
	for namespace, ns := range b.namespaces {
		ns.m.Lock()
		err := ns.ns.Release(ctx)
		if err != nil {
			releaseErr[namespace] = err.Error()
		}
		ns.clients = map[string]map[string]acquireType{}
		ns.m.Unlock()
	}
	if len(releaseErr) > 0 {
		stat, _ := status.New(codes.Unknown, "error occurred during release namespaces").
			WithDetails(&errdetails.ErrorInfo{
				Reason:   "release failure",
				Domain:   "beacon",
				Metadata: releaseErr,
			})
		return stat.Err()
	}
	return nil
}
