package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/utils"
)

func addTargetToServiceInterface(service *types.ServiceInterface, target *types.ServiceInterfaceTarget) {
	modified := false
	targets := []types.ServiceInterfaceTarget{}
	for _, t := range service.Targets {
		if t.Name == target.Name {
			modified = true
			targets = append(targets, *target)
		} else {
			targets = append(targets, t)
		}
	}
	if !modified {
		targets = append(targets, *target)
	}
	service.Targets = targets
}

func getServiceInterfaceTarget(targetType string, targetName string, deducePort bool, cli *VanClient) (*types.ServiceInterfaceTarget, error) {
	// note: selector will indicate targetType
	if targetType == "container" {
		containerJSON, err := docker.InspectContainer(targetName, cli.DockerInterface)
		if err == nil {
			target := types.ServiceInterfaceTarget{
				Name:     targetName,
				Selector: "internal.skupper.io/container",
			}
			if deducePort {
				if len(containerJSON.Config.ExposedPorts) > 0 {
					//TODO get port from config
					// a map of nat port sets, how to choose a port?
					target.TargetPort = 9090
				}
			}
			return &target, nil
		} else {
			return nil, fmt.Errorf("Could not read container %s: %s", targetName, err)
		}
	} else if targetType == "host-service" {
		// add ip if not provided
		name := targetName
		arr := strings.SplitN(name, ":", 2)
		if len(arr) == 1 {
			host := utils.GetInternalIP("docker0")
			if host == "" {
				host = "172.17.0.1"
			}
			name = targetName + ":" + host
		}
		target := types.ServiceInterfaceTarget{
			Name:     name,
			Selector: "internal.skupper.io/host-service",
		}
		// TODO: is there any way to deduce a port for a host-service
		return &target, nil
	} else {
		return nil, fmt.Errorf("VAN service interface unsupported target type")
	}
}

func updateServiceInterface(service *types.ServiceInterface, overwriteIfExists bool, cli *VanClient) error {
	current := make(map[string]types.ServiceInterface)

	svcFile, err := ioutil.ReadFile(types.GetSkupperPath(types.ServicesPath) + "/skupper-services")
	if err != nil {
		return fmt.Errorf("Failed to retrieve skupper service interace definitions: %w", err)
	}

	err = json.Unmarshal([]byte(svcFile), &current)
	if err != nil {
		return fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}

	_, ok := current[service.Address]
	if overwriteIfExists || !ok {
		current[service.Address] = *service
	}

	encoded, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("Failed to encode json for service interface: %w", err)
	}

	err = ioutil.WriteFile(types.GetSkupperPath(types.ServicesPath)+"/skupper-services", encoded, 0755)
	if err != nil {
		return fmt.Errorf("Failed to write service interface file: %w", err)
	}
	return nil
}

func validateServiceInterface(service *types.ServiceInterface) error {

	for _, target := range service.Targets {
		if target.TargetPort < 0 || 65535 < target.TargetPort {
			return fmt.Errorf("Bad target port number. Target: %s  Port: %d", target.Name, target.TargetPort)
		}
	}

	//TODO: change service.Protocol to service.Mapping
	if service.Port < 0 || 65535 < service.Port {
		return fmt.Errorf("Port %d is outside valid range.", service.Port)
	} else if service.Aggregate != "" && service.EventChannel {
		return fmt.Errorf("Only one of aggregate and event-channel can be specified for a given service.")
	} else if service.Aggregate != "" && service.Aggregate != "json" && service.Aggregate != "multipart" {
		return fmt.Errorf("%s is not a valid aggregation strategy. Choose 'json' or 'multipart'.", service.Aggregate)
	} else if service.Protocol != "" && service.Protocol != "tcp" && service.Protocol != "http" && service.Protocol != "http2" {
		return fmt.Errorf("%s is not a valid mapping. Choose 'tcp', 'http' or 'http2'.", service.Protocol)
	} else if service.Aggregate != "" && service.Protocol != "http" {
		return fmt.Errorf("The aggregate option is currently only valid for http")
	} else if service.EventChannel && service.Protocol != "http" {
		return fmt.Errorf("The event-channel option is currently only valid for http")
	} else {
		return nil
	}
}

func (cli *VanClient) ServiceInterfaceUpdate(ctx context.Context, service *types.ServiceInterface) error {
	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	_, err = cli.ServiceInterfaceInspect(service.Address)
	if err != nil {
		return fmt.Errorf("Service not found: %w", err)
	}

	err = validateServiceInterface(service)
	if err != nil {
		return err
	}
	return updateServiceInterface(service, true, cli)
}

func (cli *VanClient) ServiceInterfaceBind(service *types.ServiceInterface, targetType string, targetName string, protocol string, targetPort int) error {
	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	err = validateServiceInterface(service)
	if err != nil {
		return err
	}
	if protocol != "" && service.Protocol != protocol {
		return fmt.Errorf("Invalid protocol %s for service with mapping %s", protocol, service.Protocol)
	}
	target, err := getServiceInterfaceTarget(targetType, targetName, service.Port == 0 && targetPort == 0, cli)
	if err != nil {
		return err
	}
	if target.TargetPort != 0 {
		service.Port = target.TargetPort
		target.TargetPort = 0
	} else if targetPort != 0 {
		if service.Port == 0 {
			service.Port = targetPort
		} else {
			target.TargetPort = targetPort
		}
	}
	if service.Port == 0 {
		if protocol == "http" {
			service.Port = 80
		} else {
			return fmt.Errorf("Service port required and cannot be deduced.")
		}
	}
	addTargetToServiceInterface(service, target)
	return updateServiceInterface(service, true, cli)
}

func removeServiceInterfaceTarget(serviceName string, targetName string, deleteIfNoTargets bool, cli *VanClient) error {
	current := make(map[string]types.ServiceInterface)

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	svcFile, err := ioutil.ReadFile(types.GetSkupperPath(types.ServicesPath) + "/skupper-services")
	if err != nil {
		return fmt.Errorf("Failed to retrieve skupper service interace definitions: %w", err)
	}

	err = json.Unmarshal([]byte(svcFile), &current)
	if err != nil {
		return fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}

	if _, ok := current[serviceName]; !ok {
		return fmt.Errorf("Could not find entry for service interface %s", serviceName)
	}

	service := current[serviceName]
	modified := false
	targets := []types.ServiceInterfaceTarget{}

	for _, t := range service.Targets {
		name := targetName
		if t.Selector == "internal.skupper.io/host-service" {
			// add ip if not provided
			arr := strings.SplitN(name, ":", 2)
			if len(arr) == 1 {
				// magic address
				host := utils.GetInternalIP("docker0")
				if host == "" {
					host = "172.17.0.1"
				}
				name = targetName + ":" + host
			}
		}
		if t.Name == name || (t.Name == "" && targetName == serviceName) {
			modified = true
		} else {
			targets = append(targets, t)
		}
	}
	if !modified {
		return fmt.Errorf("Could not find target %s for service interface %s", targetName, serviceName)
	}
	if len(targets) == 0 && deleteIfNoTargets {
		delete(current, serviceName)
	} else {
		service.Targets = targets
		current[serviceName] = service
	}

	encoded, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("Failed to encode json for service interface: %w", err)
	}

	err = ioutil.WriteFile(types.GetSkupperPath(types.ServicesPath)+"/skupper-services", encoded, 0755)
	if err != nil {
		return fmt.Errorf("Failed to write service interface file: %w", err)
	}
	return nil
}

func (cli *VanClient) ServiceInterfaceUnbind(targetType string, targetName string, address string, deleteIfNoTargets bool) error {
	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	if targetType == "container" || targetType == "host-service" {
		err := removeServiceInterfaceTarget(address, targetName, deleteIfNoTargets, cli)
		return err
	} else {
		return fmt.Errorf("Unsupported target type for service interface %s", targetType)
	}
}
