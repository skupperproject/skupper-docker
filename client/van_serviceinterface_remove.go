package client

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) VanServiceInterfaceRemove(address string) error {

	// TODO: check that all options are present

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve transport container ", err.Error())
		return err
	}

	// check that a service with that name already has been attached to the VAN
	_, err = ioutil.ReadFile(types.ServicePath + address)
	if err != nil {
		log.Println("Service interface for address does not exist", address)
		return err
	}

	err = os.Remove(types.ServicePath + address)
	if err != nil {
		log.Println("Failed to remove service interface file", err.Error())
	}

	return err
}
