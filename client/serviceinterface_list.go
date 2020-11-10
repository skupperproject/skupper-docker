package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) ServiceInterfaceList() ([]types.ServiceInterface, error) {
	var vsis []types.ServiceInterface
	svcDefs := make(map[string]types.ServiceInterface)

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	//	svcFile, err := ioutil.ReadFile(types.AllServiceDefsFile)
	svcFile, err := ioutil.ReadFile(types.ServiceDefsFile)
	if err != nil {
		return vsis, err
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return vsis, fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}
	for _, v := range svcDefs {
		current, err := docker.InspectContainer(v.Address, cli.DockerInterface)
		if err == nil {
			v.Alias = string(current.NetworkSettings.Networks["skupper-network"].IPAddress)
		}
		vsis = append(vsis, v)
	}

	return vsis, err
}
