package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) VanServiceInterfaceList() ([]types.ServiceInterface, error) {
	var vsis []types.ServiceInterface
	svcDefs := make(map[string]types.ServiceInterface)

	svcFile, err := ioutil.ReadFile(types.AllServiceDefsFile)
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
