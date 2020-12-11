package client

import (
	"fmt"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) ServiceInterfaceCreate(service *types.ServiceInterface) error {
	//func (cli *VanClient) ServiceInterfaceCreate(targetType string, targetName string, options types.ServiceInterfaceCreateOptions) error {

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	err = validateServiceInterface(service)
	if err != nil {
		return err
	}
	return updateServiceInterface(service, false, cli)

}
