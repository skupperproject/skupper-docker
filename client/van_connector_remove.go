package client

import (
	"log"
	"os"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) VanConnectorRemove(name string) error {

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve transport container", err.Error())
	}

	// TODO  should we check inf connector actually exists to indicate it is not found
	err = os.RemoveAll(types.ConnPath + name)
	if err != nil {
		log.Println("Failed to remove connector file contents", err.Error())
	}

	err = docker.RestartTransportContainer(cli.DockerInterface)
	if err != nil {
		log.Println("Failed to restart transport container", err.Error())
	}

	err = docker.RestartControllerContainer(cli.DockerInterface)
	if err != nil {
	    log.Println("Failed to restart controller container", err.Error())
	}

	return err
}
