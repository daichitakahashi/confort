package confort

import (
	"context"

	"github.com/daichitakahashi/confort/compose"
	"github.com/docker/go-connections/nat"
)

type (
	// Backend implementation using Docker Compose CLI.
	composeBackend struct{}

	composer struct {
		proj projectConfig
	}

	/*
		{
		  "name": "compose-scale",
		  "services": {
		    "observer": {
		      "command": [
		        "sleep",
		        "infinity"
		      ],
		      "entrypoint": null,
		      "image": "alpine:3.16.2",
		      "networks": {
		        "default": null
		      }
		    },
		    "server": {
		      "command": null,
		      "deploy": {
		        "mode": "replicated",
		        "replicas": 3,
		        "resources": {},
		        "placement": {},
		        "endpoint_mode": "vip"
		      },
		      "entrypoint": null,
		      "environment": {
		        "HOGE": "VALUE"
		      },
		      "image": "nginx:1.23.2",
		      "networks": {
		        "default": null
		      },
		      "ports": [
		        {
		          "mode": "ingress",
		          "target": 80,
		          "protocol": "tcp"
		        }
		      ]
		    }
		  },
		  "networks": {
		    "default": {
		      "name": "compose-scale_default",
		      "ipam": {},
		      "external": false
		    }
		  }
		}
	*/
	projectConfig struct {
		Name        string
		ConfigFiles []string
		Services    []struct {
			// services that has been enabled by profiles
			Name         string
			ExposedPorts nat.PortMap
		}
	}
)

func (composeBackend) Load(ctx context.Context, projectDir string, configFiles []string, envFile *string, profiles []string) (compose.Composer, error) {
	return &composer{}, nil
}

var _ compose.Backend = (*composeBackend)(nil)

func (c composer) Up(ctx context.Context, service string, opts compose.UpOptions) (*compose.Service, error) {
	//TODO implement me
	panic("implement me")
}

func (c composer) Down(ctx context.Context, services []string) error {
	//TODO implement me
	panic("implement me")
}

var _ compose.Composer = (*composer)(nil)
