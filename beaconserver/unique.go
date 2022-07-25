package beaconserver

import (
	"context"
	"sync"

	"github.com/daichitakahashi/confort/proto/beacon"
)

type uniqueValueServer struct {
	beacon.UnimplementedUniqueValueServiceServer
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

func (u *uniqueValueServer) StoreUniqueValue(_ context.Context, req *beacon.StoreUniqueValueRequest) (*beacon.StoreUniqueValueResponse, error) {
	v, _ := u.stores.LoadOrStore(req.GetStore(), &valueStore{})
	store := v.(*valueStore)
	return &beacon.StoreUniqueValueResponse{
		Succeeded: store.tryStore(req.GetValue()),
	}, nil
}

var _ beacon.UniqueValueServiceServer = (*uniqueValueServer)(nil)
