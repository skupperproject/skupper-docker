package client

import (
	"fmt"
	"os"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) ConnectorRemove(name string) error {

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

	err = docker.RestartContainer(types.ControllerDeploymentName, cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to restart controller container: %w", err)
	}

	// restart proxies
	vsis, err := cli.ServiceInterfaceList()
	if err != nil {
		return fmt.Errorf("Failed to list proxies to restart: %w", err)
	}
	for _, vs := range vsis {
		err = docker.RestartContainer(vs.Address, cli.DockerInterface)
		if err != nil {
			return fmt.Errorf("Failed to restart proxy container: %w", err)
		}
	}

	return nil
}
