package client

import (
	"encoding/json"
	"io/ioutil"
	"log"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func (cli *VanClient) VanServiceInterfaceCreate(targetName string, options types.VanServiceInterfaceCreateOptions) error {

	// TODO: check that all options are present

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve transport container (need init?): ", err.Error())
		return err
	}

	// check that a service with that name already has been attached to the VAN
	_, err = ioutil.ReadFile(types.ServicePath + options.Address)
	if err == nil {
		// TODO: Deal with update case , read in json file, decode and update
		log.Printf("Expose target name %s already exists\n", targetName)
		return err
	}

	if targetName == options.Address {
		log.Println("the exposed address and container target name must be different")
		return nil
	}

	_, err = docker.InspectContainer(targetName, cli.DockerInterface)
	if err != nil {
		// TODO: handle exited, not running, is not found etc.
		log.Println("Error retrieving service target container", err.Error())
		return err
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

	encoded, err := json.Marshal(serviceInterface)
	if err != nil {
		log.Println("Failed to create json for service interface", err.Error())
		return err
	}

	err = ioutil.WriteFile(types.ServicePath+options.Address, encoded, 0755)
	if err != nil {
		log.Println("Failed to write service interface file", err.Error())
	}

	return nil
}
