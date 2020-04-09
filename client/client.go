package client

import (
	"time"

	"github.com/skupperproject/skupper-docker/pkg/docker/libdocker"
)

// A VAN client manages orchestration and communication with the network components
type VanClient struct {
	DockerInterface libdocker.Interface
}

func NewClient(endpoint string) (*VanClient, error) {
	c := &VanClient{}

	c.DockerInterface = libdocker.ConnectToDockerOrDie(endpoint, 0, 10*time.Second)

	return c, nil
}
