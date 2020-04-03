package client

import (
	"log"
	"os"

	dockertypes "github.com/docker/docker/api/types"
	dockerfilters "github.com/docker/docker/api/types/filters"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

//TODO should there be remove options
//TODO should not have docker imports here?

// VanRouterRemove delete a VAN (transport and controller) deployment
func (cli *VanClient) VanRouterRemove() error {

	// stop controller
	err := docker.StopContainer(types.ControllerDeploymentName, cli.DockerInterface)
	if err != nil {
		log.Println("Could not stop controller container", err.Error())
	}
	err = docker.RemoveContainer(types.ControllerDeploymentName, cli.DockerInterface)
	if err != nil {
		log.Println("Could not remove controller container", err.Error())
	}

	// remove proxies
	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/component")
	opts := dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	}
	containers, err := docker.ListContainers(opts, cli.DockerInterface)
	if err != nil {
		log.Println("Failed to list proxy containers", err.Error())
	} else {
		for _, container := range containers {
			if value, ok := container.Labels["skupper.io/component"]; ok {
				if value == "proxy" {
					err := docker.StopContainer(container.ID, cli.DockerInterface)
					if err != nil {
						log.Println("Failed to stop proxy container", err.Error())
					}
					err = docker.RemoveContainer(container.ID, cli.DockerInterface)
					if err != nil {
						log.Println("Failed to remove proxy container", err.Error())
					}
				}
			}
		}
	}

	// stop transport
	err = docker.StopContainer(types.TransportDeploymentName, cli.DockerInterface)
	if err != nil {
		log.Println("Could not stop transport container", err.Error())
	}
	err = docker.RemoveContainer(types.TransportDeploymentName, cli.DockerInterface)
	if err != nil {
		log.Println("Could not remove controller container", err.Error())
	}

	// remove network
	err = docker.RemoveNetwork("skupper-network", cli.DockerInterface)
	if err != nil {
		log.Println("Could not remove network", err.Error())
	}

	// remove host files
	err = os.RemoveAll(types.HostPath)
	if err != nil {
		log.Println("Failed to remove skupper files and directory: ", err.Error())
	}

	return nil
}
