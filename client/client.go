package client

import (
	"time"

	//	dockerapi "github.com/docker/docker/client"
	//  	"golang.org/x/net/context"

	//"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/dockershim/libdocker"
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
