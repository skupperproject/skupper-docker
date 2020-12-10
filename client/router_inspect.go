package client

import (
	"fmt"
	"log"
	"time"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func (cli *VanClient) RouterInspect() (*types.RouterInspectResponse, error) {
	vir := &types.RouterInspectResponse{}

	transport, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve transport container (need init?): ", err.Error())
		return vir, err
	}

	vir.TransportVersion, err = docker.GetImageVersion(transport.Config.Image, cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve transport container version:", err.Error())
		return vir, err
	}
	vir.Status.State = transport.State.Status

	controller, err := docker.InspectContainer(types.ControllerDeploymentName, cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve controller container (need init?): ", err.Error())
		return vir, err
	}

	vir.ControllerVersion, err = docker.GetImageVersion(controller.Config.Image, cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve controller container version:", err.Error())
		return vir, err
	}

	routerConfig, err := qdr.GetRouterConfigFromFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return vir, fmt.Errorf("Failed to retrieve router config: %w", err)
	}
	vir.Status.Mode = string(routerConfig.Metadata.Mode)

	connected, err := qdr.GetConnectedSites(cli.DockerInterface)
	for i := 0; i < 5 && err != nil; i++ {
		time.Sleep(500 * time.Millisecond)
		connected, err = qdr.GetConnectedSites(cli.DockerInterface)
	}
	if err != nil {
		return vir, err
	}
	vir.Status.ConnectedSites = connected

	vsis, err := cli.ServiceInterfaceList()
	if err != nil {
		vir.ExposedServices = 0
	} else {
		vir.ExposedServices = len(vsis)
	}

	return vir, err
}
