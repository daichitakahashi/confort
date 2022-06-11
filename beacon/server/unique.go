package server

import (
	"context"

	"github.com/daichitakahashi/confort/proto/beacon"
)

type uniqueValueServer struct {
	beacon.UnimplementedUniqueValueServiceServer
}

func (u *uniqueValueServer) StoreUniqueValue(ctx context.Context, request *beacon.StoreUniqueValueRequest) (*beacon.StoreUniqueValueResponse, error) {
	// TODO implement me
	panic("implement me")
}

var _ beacon.UniqueValueServiceServer = (*uniqueValueServer)(nil)
