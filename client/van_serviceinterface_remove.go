package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) VanServiceInterfaceRemove(targetType string, targetName string, options types.VanServiceInterfaceRemoveOptions) error {
	// TODO: check that all options are present

	svcDefs := make(map[string]types.ServiceInterface)

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	svcFile, err := ioutil.ReadFile(types.LocalSifs)
	if err != nil {
		return fmt.Errorf("Failed to retrieve skupper service interace definitions: %w", err)
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}

	if _, ok := svcDefs[options.Address]; !ok {
		return fmt.Errorf("Unexpose service interface definition not found")
	}

	delete(svcDefs, options.Address)

	encoded, err := json.Marshal(svcDefs)

	if err != nil {
		return fmt.Errorf("Failed to encode json for service interface: %w", err)
	}

	err = ioutil.WriteFile(types.LocalSifs, encoded, 0755)
	if err != nil {
		return fmt.Errorf("Failed to write service interface file: %w", err)
	}

	return nil
}
