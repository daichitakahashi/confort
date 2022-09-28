package server

import (
	"context"
	"sync"

	"github.com/daichitakahashi/confort/internal/beacon/proto"
)

type uniqueValueServer struct {
	proto.UnimplementedUniqueValueServiceServer
	stores sync.Map
}

type valueStore struct {
	m      sync.Mutex
	values map[string]bool
}

func (s *valueStore) tryStore(v string) bool {
	s.m.Lock()
	defer s.m.Unlock()

	// lazy init
	if s.values == nil {
		s.values = map[string]bool{}
	}

	// check if value is existing
	if s.values[v] {
		return false
	}
	// set v as new unique value
	s.values[v] = true
	return true
}

func (u *uniqueValueServer) StoreUniqueValue(_ context.Context, req *proto.StoreUniqueValueRequest) (*proto.StoreUniqueValueResponse, error) {
	v, _ := u.stores.LoadOrStore(req.GetStore(), &valueStore{})
	store := v.(*valueStore)
	return &proto.StoreUniqueValueResponse{
		Succeeded: store.tryStore(req.GetValue()),
	}, nil
}

var _ proto.UniqueValueServiceServer = (*uniqueValueServer)(nil)
