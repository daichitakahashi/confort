package confort

import (
	"testing"

	"github.com/docker/go-connections/nat"
)

var ports = Ports{
	"80/tcp": []nat.PortBinding{
		{
			HostIP:   "127.0.0.1",
			HostPort: "32768",
		}, {
			HostIP:   "127.0.0.1",
			HostPort: "49870",
		},
	},
}

func assertEqual[T comparable](t *testing.T, expected, actual T) {
	t.Helper()
	if expected != actual {
		t.Fatalf("want %v, got %v", expected, actual)
	}
}

func TestPorts_Binding(t *testing.T) {
	t.Parallel()

	testCases := map[nat.Port]nat.PortBinding{
		"80/tcp": {
			HostIP:   "127.0.0.1",
			HostPort: "32768",
		},
		"8080/tcp": {},
	}
	for port, expected := range testCases {
		v := ports.Binding(port)
		assertEqual(t, expected, v)
	}
}

func TestPorts_HostPort(t *testing.T) {
	t.Parallel()

	testCases := map[nat.Port]string{
		"80/tcp":   "127.0.0.1:32768",
		"8080/tcp": "",
	}
	for port, expected := range testCases {
		v := ports.HostPort(port)
		assertEqual(t, expected, v)
	}
}

func TestPorts_URL(t *testing.T) {
	t.Parallel()

	t.Run("http(default)", func(t *testing.T) {
		testCases := map[nat.Port]string{
			"80/tcp":   "http://127.0.0.1:32768",
			"8080/tcp": "",
		}
		for port, expected := range testCases {
			v := ports.URL(port, "")
			assertEqual(t, expected, v)
		}
	})

	t.Run("ftp", func(t *testing.T) {
		testCases := map[nat.Port]string{
			"80/tcp":   "ftp://127.0.0.1:32768",
			"8080/tcp": "",
		}
		for port, expected := range testCases {
			v := ports.URL(port, "ftp")
			assertEqual(t, expected, v)
		}
	})
}
