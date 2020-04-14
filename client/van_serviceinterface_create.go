package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/utils"
)

func (cli *VanClient) VanServiceInterfaceCreate(targetType string, targetName string, options types.VanServiceInterfaceCreateOptions) error {
	svcDefs := make(map[string]types.ServiceInterface)

	// TODO: check that all options are present
	// TODO: don't expose same container twice

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

	if _, ok := svcDefs[options.Address]; ok {
		// TODO: Deal with update case , read in json file, decode and update
		return fmt.Errorf("Expose target name %s already exists\n", targetName)
	}

	if targetType == "container" {
		if targetName == options.Address {
			return fmt.Errorf("the exposed address and container target name must be different")
		}

		_, err = docker.InspectContainer(targetName, cli.DockerInterface)
		if err != nil {
			// TODO: handle exited, not running, is not found etc.
			return fmt.Errorf("Error retrieving service target container: %w", err)
		}

		serviceInterfaceTarget := types.ServiceInterfaceTarget{
			Name:       targetName,
			Selector:   "",
			TargetPort: options.TargetPort,
		}

		serviceInterface := types.ServiceInterface{
			Address:  options.Address,
			Protocol: options.Protocol,
			Port:     options.Port,
			Targets: []types.ServiceInterfaceTarget{
				serviceInterfaceTarget,
			},
		}

		svcDefs[options.Address] = serviceInterface

		encoded, err := json.Marshal(svcDefs)
		if err != nil {
			return fmt.Errorf("Failed to encode json for service interface: %w", err)
		}

		err = ioutil.WriteFile(types.LocalSifs, encoded, 0755)
		if err != nil {
			return fmt.Errorf("Failed to write service interface file: %w", err)
		}
	} else if targetType == "host" {
		if options.Port == 0 {
			return fmt.Errorf("Host service must specify port, use --port option to provide it")
		}
		hostIP := utils.GetInternalIP(targetName)
		if hostIP == "" {
			return fmt.Errorf("Error retrieving host target network address")
		}

		serviceInterfaceTarget := types.ServiceInterfaceTarget{
			Name:       hostIP,
			Selector:   "internal.skupper.io/hostservice",
			TargetPort: options.TargetPort,
		}

		serviceInterface := types.ServiceInterface{
			Address:  options.Address,
			Protocol: options.Protocol,
			Port:     options.Port,
			Targets: []types.ServiceInterfaceTarget{
				serviceInterfaceTarget,
			},
		}

		svcDefs[options.Address] = serviceInterface

		encoded, err := json.Marshal(svcDefs)
		if err != nil {
			return fmt.Errorf("Failed to encode json for service interface: %w", err)
		}

		err = ioutil.WriteFile(types.LocalSifs, encoded, 0755)
		if err != nil {
			return fmt.Errorf("Failed to write service interface file: %w", err)
		}

	} else {
		return fmt.Errorf("Expose target type %s not supported", targetType)
	}

	return nil
}
