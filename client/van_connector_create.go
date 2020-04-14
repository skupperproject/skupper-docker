package client

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

	"github.com/skupperproject/skupper-cli/pkg/certs"
	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
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

func (cli *VanClient) VanConnectorCreate(secretFile string, options types.VanConnectorCreateOptions) error {

	// TODO certs should return err
	secret := certs.GetSecretContent(secretFile)
	if secret == nil {
		return fmt.Errorf("Failed to make connector, missing connection-token content")
	}

	existing, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	mode := qdr.GetTransportMode(existing)

	if options.Name == "" {
		options.Name, err = generateConnectorName(types.ConnPath)
		if err != nil {
			return err
		}
	}
	connPath := types.ConnPath + options.Name

	if err := os.Mkdir(connPath, 0755); err != nil {
		return fmt.Errorf("Failed to create skupper connector directory: %w", err)
	}
	for k, v := range secret {
		if err := ioutil.WriteFile(connPath+"/"+k, v, 0755); err != nil {
			return fmt.Errorf("Failed to write connector certificate file: %w", err)
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
		return fmt.Errorf("Failed to re-start transport container: %w", err)
	}

	err = docker.RestartContainer("skupper-proxy-controller", cli.DockerInterface)
	//	err = docker.RestartControllerContainer(cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Failed to re-start controller container: %w", err)
	}

	return err
}
