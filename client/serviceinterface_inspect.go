package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
)

func (cli *VanClient) ServiceInterfaceInspect(address string) (*types.ServiceInterface, error) {
	svcDefs := make(map[string]types.ServiceInterface)

	svcFile, err := ioutil.ReadFile(types.AllServiceDefsFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}
	if vsi, ok := svcDefs[address]; !ok {
		return nil, fmt.Errorf("Service Interface not found: ", address)
	} else {
		return &vsi, nil
	}
}
