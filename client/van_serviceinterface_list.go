package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
)

func (cli *VanClient) VanServiceInterfaceList() ([]*types.ServiceInterface, error) {
	var vsis []*types.ServiceInterface

	svcDefs := make(map[string]types.ServiceInterface)

	svcFile, err := ioutil.ReadFile(types.AllSifs)
	if err != nil {
		return vsis, err
	}

	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return vsis, fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}
	fmt.Println("Svc defs: ", svcDefs)

	for _, v := range svcDefs {
		fmt.Println("V:", v)
		vsis = append(vsis, &v)
	}

	return vsis, err
}
