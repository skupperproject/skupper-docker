package client

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
	"github.com/skupperproject/skupper/pkg/certs"
)

func generateConnectorName(path string) (string, error) {
	files, err := ioutil.ReadDir(path)
	max := 1
	if err == nil {
		connectorNamePattern := regexp.MustCompile("conn([0-9])+")
		for _, f := range files {
			count := connectorNamePattern.FindStringSubmatch(f.Name())
			if len(count) > 1 {
				v, _ := strconv.Atoi(count[1])
				if v >= max {
					max = v + 1
				}
			}
		}
	} else {
		return "", fmt.Errorf("Could not retrieve configured connectors (need init?): %w", err)
	}
	return "conn" + strconv.Itoa(max), nil
}

func (cli *VanClient) ConnectorCreate(secretFile string, options types.ConnectorCreateOptions) (string, error) {

	// TODO certs should return err
	secret, err := certs.GetSecretContent(secretFile)
	if err != nil {
		return "", fmt.Errorf("Failed to make connector: %w", err)
	}

	existing, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return "", fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	generatedBy, ok := secret["skupper.io/generated-by"]
	if !ok {
		return "", fmt.Errorf("Cannot find secret origin for token '%s'", secretFile)
	}

	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return "", fmt.Errorf("Unable to retrieve site UUID: %w", err)
	}

	if sc.UID == string(generatedBy) {
		return "", fmt.Errorf("Cannot create connection to self with token '%s'", secretFile)
	}

	mode := qdr.GetTransportMode(existing)

	if options.Name == "" {
		options.Name, err = generateConnectorName(types.ConnPath)
		if err != nil {
			return "", err
		}
	}
	connPath := types.ConnPath + options.Name

	if err := os.Mkdir(connPath, 0755); err != nil {
		return "", fmt.Errorf("Failed to create skupper connector directory: %w", err)
	}
	for k, v := range secret {
		if k == types.TokenGeneratedBy {
			if err := ioutil.WriteFile(connPath+"/generated-by", v, 0755); err != nil {
				return "", fmt.Errorf("Failed to write connector file: %w", err)
			}
		} else {
			if err := ioutil.WriteFile(connPath+"/"+k, v, 0755); err != nil {
				return "", fmt.Errorf("Failed to write connector certificate file: %w", err)
			}
		}
	}

	connector := types.Connector{
		Name: options.Name,
		Cost: options.Cost,
	}
	if mode == types.TransportModeInterior {
		hostString, _ := ioutil.ReadFile(connPath + "/inter-router-host")
		portString, _ := ioutil.ReadFile(connPath + "/inter-router-port")
		connector.Host = string(hostString)
		connector.Port = string(portString)
		connector.Role = string(types.ConnectorRoleInterRouter)
	} else {
		hostString, _ := ioutil.ReadFile(connPath + "/edge-host")
		portString, _ := ioutil.ReadFile(connPath + "/edge-port")
		connector.Host = string(hostString)
		connector.Port = string(portString)
		connector.Role = string(types.ConnectorRoleEdge)
	}

	err = docker.RestartTransportContainer(cli.DockerInterface)
	if err != nil {
		return "", fmt.Errorf("Failed to re-start transport container: %w", err)
	}

	err = docker.RestartContainer(types.ControllerDeploymentName, cli.DockerInterface)
	if err != nil {
		return "", fmt.Errorf("Failed to re-start controller container: %w", err)
	}

	// restart proxies
	vsis, err := cli.ServiceInterfaceList()
	if err != nil {
		return "", fmt.Errorf("Failed to list proxies to restart: %w", err)
	}
	for _, vs := range vsis {
		err = docker.RestartContainer(vs.Address, cli.DockerInterface)
		if err != nil {
			return "", fmt.Errorf("Failed to restart proxy container: %w", err)
		}
	}

	return options.Name, nil
}
