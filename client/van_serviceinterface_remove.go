package client

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) VanServiceInterfaceRemove(address string) error {

	// TODO: check that all options are present

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	// check that a service with that name already has been attached to the VAN
	_, err = ioutil.ReadFile(types.ServicePath + address)
	if err != nil {
		return fmt.Errorf("Failed to retrieve service interface: %w", err)
	}

	err = os.Remove(types.ServicePath + address)
	if err != nil {
		return fmt.Errorf("Failed to remove service interface file: %w", err)
	}

	return nil
}
