package client

import (
	"fmt"
	"os"

	dockertypes "github.com/docker/docker/api/types"
	dockerfilters "github.com/docker/docker/api/types/filters"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

//TODO should there be remove options

// VanRouterRemove delete a VAN (transport and controller) deployment
func (cli *VanClient) VanRouterRemove() []error {
	results := []error{}

	_, err := docker.InspectContainer(types.ControllerDeploymentName, cli.DockerInterface)
	if err == nil {
		// stop controller
		err = docker.StopContainer(types.ControllerDeploymentName, cli.DockerInterface)
		if err != nil {
			results = append(results, fmt.Errorf("Could not stop controller container: %w", err))
		} else {
			err = docker.RemoveContainer(types.ControllerDeploymentName, cli.DockerInterface)
			if err != nil {
				results = append(results, fmt.Errorf("Could not remove controller container: %w", err))
			}
		}
	}

	// remove proxies
	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/component")
	opts := dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	}
	containers, err := docker.ListContainers(opts, cli.DockerInterface)
	if err == nil {
		for _, container := range containers {
			if value, ok := container.Labels["skupper.io/component"]; ok {
				if value == "proxy" {
					err := docker.StopContainer(container.ID, cli.DockerInterface)
					if err != nil {
						results = append(results, fmt.Errorf("Failed to stop proxy container: %w", err))
					} else {
						err = docker.RemoveContainer(container.ID, cli.DockerInterface)
						if err != nil {
							results = append(results, fmt.Errorf("Failed to remove proxy container: %w", err))
						}
					}
				}
			}
		}
	} else {
		results = append(results, fmt.Errorf("Failed to list proxy containers: %w", err))
	}

	_, err = docker.InspectContainer(types.TransportDeploymentName, cli.DockerInterface)
	if err == nil {
		// stop transport
		err = docker.StopContainer(types.TransportDeploymentName, cli.DockerInterface)
		if err != nil {
			results = append(results, fmt.Errorf("Could not stop transport container: %w", err))
		} else {
			err = docker.RemoveContainer(types.TransportDeploymentName, cli.DockerInterface)
			if err != nil {
				results = append(results, fmt.Errorf("Could not remove controller container: %w", err))
			}
		}
	}

	_, err = docker.InspectNetwork("skupper-network", cli.DockerInterface)
	if err == nil {
		// remove network
		err = docker.RemoveNetwork("skupper-network", cli.DockerInterface)
		if err != nil {
			results = append(results, fmt.Errorf("Could not remove skupper network: %w", err))
		}
	}

	// remove host files
	err = os.RemoveAll(types.HostPath)
	if err != nil {
		results = append(results, fmt.Errorf("Failed to remove skupper files and directory: %w", err))
	}

	return results
}
