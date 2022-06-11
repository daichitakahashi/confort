package server

import (
	"context"

	"github.com/daichitakahashi/confort/proto/beacon"
	"google.golang.org/protobuf/types/known/emptypb"
)

type beaconServer struct {
	beacon.UnimplementedBeaconServiceServer
}

func (b beaconServer) Register(ctx context.Context, request *beacon.RegisterRequest) (*beacon.RegisterResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (b beaconServer) Deregister(ctx context.Context, request *beacon.DeregisterRequest) (*emptypb.Empty, error) {
	// TODO implement me
	panic("implement me")
}

func (b beaconServer) BuildImage(request *beacon.BuildImageRequest, server beacon.BeaconService_BuildImageServer) error {
	// TODO implement me
	panic("implement me")
}

func (b beaconServer) CreateContainer(request *beacon.CreateContainerRequest, server beacon.BeaconService_CreateContainerServer) error {
	// TODO implement me
	panic("implement me")
}

func (b beaconServer) AcquireContainerEndpoint(request *beacon.AcquireContainerEndpointRequest, server beacon.BeaconService_AcquireContainerEndpointServer) error {
	// TODO implement me
	panic("implement me")
}

func (b beaconServer) ReleaseContainer(ctx context.Context, request *beacon.ReleaseContainerRequest) (*emptypb.Empty, error) {
	// TODO implement me
	panic("implement me")
}

var _ beacon.BeaconServiceServer = (*beaconServer)(nil)
