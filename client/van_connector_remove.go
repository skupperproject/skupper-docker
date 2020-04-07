package client

import (
	"fmt"
	"os"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) VanConnectorRemove(name string) error {

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container: %w", err)
	}

	// TODO  should we check inf connector actually exists to indicate it is not found
	err = os.RemoveAll(types.ConnPath + name)
	if err != nil {
		return fmt.Errorf("Failed to remove connector file contents: %w", err)
	}

	err = docker.RestartTransportContainer(cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to restart transport container: %w", err)
	}

	err = docker.RestartControllerContainer(cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to restart controller container: %w", err)
	}

	return nil
}
