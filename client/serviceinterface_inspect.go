package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) ServiceInterfaceInspect(address string) (*types.ServiceInterface, error) {
	svcDefs := make(map[string]types.ServiceInterface)

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	//	svcFile, err := ioutil.ReadFile(types.AllServiceDefsFile)
	svcFile, err := ioutil.ReadFile(types.ServiceDefsFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}
	if vsi, ok := svcDefs[address]; !ok {
		return nil, nil
	} else {
		return &vsi, nil
	}
}
