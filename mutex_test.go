package confort_test

import (
	"net"
	"testing"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/beaconserver"
	"github.com/daichitakahashi/confort/internal/mutextest"
	"github.com/daichitakahashi/confort/proto/beacon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newBeaconControl(t *testing.T) confort.ExclusionControl {
	t.Helper()

	srv := grpc.NewServer()
	beaconserver.Register(srv, func() error {
		return nil
	})

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srv.Serve(ln)
		_ = ln.Close()
	}()
	t.Cleanup(func() {
		srv.Stop()
	})

	conn, err := grpc.Dial(ln.Addr().String(), grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return confort.NewBeaconControl(beacon.NewBeaconServiceClient(conn))
}

func TestExclusionControl_NamespaceLock(t *testing.T) {
	t.Parallel()

	t.Run("without beacon", func(t *testing.T) {
		t.Parallel()
		ex := confort.NewExclusionControl()
		mutextest.TestNamespaceLock(t, ex)
	})
	t.Run("with beacon", func(t *testing.T) {
		t.Parallel()
		ex := newBeaconControl(t)
		mutextest.TestNamespaceLock(t, ex)
	})
}

func TestExclusionControl_BuildLock(t *testing.T) {
	t.Parallel()

	t.Run("without beacon", func(t *testing.T) {
		t.Parallel()
		ex := confort.NewExclusionControl()
		mutextest.TestBuildLock(t, ex)
	})
	t.Run("with beacon", func(t *testing.T) {
		t.Parallel()
		ex := newBeaconControl(t)
		mutextest.TestBuildLock(t, ex)
	})
}

func TestExclusionControl_InitContainerLock(t *testing.T) {
	t.Parallel()

	t.Run("without beacon", func(t *testing.T) {
		t.Parallel()
		ex := confort.NewExclusionControl()
		mutextest.TestInitContainerLock(t, ex)
	})
	t.Run("with beacon", func(t *testing.T) {
		t.Parallel()
		ex := newBeaconControl(t)
		mutextest.TestInitContainerLock(t, ex)
	})
}

func TestExclusionControl_AcquireContainerLock(t *testing.T) {
	t.Parallel()

	t.Run("without beacon", func(t *testing.T) {
		t.Parallel()
		ex := confort.NewExclusionControl()
		mutextest.TestAcquireContainerLock(t, ex)
	})
	t.Run("with beacon", func(t *testing.T) {
		t.Parallel()
		ex := newBeaconControl(t)
		mutextest.TestAcquireContainerLock(t, ex)
	})
}

func TestExclusionControl_TryAcquireContainerInitLock(t *testing.T) {
	t.Parallel()

	t.Run("acquire without beacon", func(t *testing.T) {
		t.Parallel()
		ex := confort.NewExclusionControl()
		mutextest.TestTryAcquireContainerInitLock(t, ex, true)
	})
	t.Run("acquire with beacon", func(t *testing.T) {
		t.Parallel()
		ex := newBeaconControl(t)
		mutextest.TestTryAcquireContainerInitLock(t, ex, true)
	})
	t.Run("no acquire without beacon", func(t *testing.T) {
		t.Parallel()
		ex := confort.NewExclusionControl()
		mutextest.TestTryAcquireContainerInitLock(t, ex, false)
	})
	t.Run("no acquire with beacon", func(t *testing.T) {
		t.Parallel()
		ex := newBeaconControl(t)
		mutextest.TestTryAcquireContainerInitLock(t, ex, false)
	})
}
