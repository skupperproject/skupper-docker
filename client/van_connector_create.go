package client

import (
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/skupperproject/skupper-cli/pkg/certs"
	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func generateConnectorName(path string) string {
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
		log.Fatal("Could not retrieve configured connectors (need init?): ", err.Error())
	}
	return "conn" + strconv.Itoa(max)
}

func (cli *VanClient) VanConnectorCreate(secretFile string, options types.VanConnectorCreateOptions) error {

	// TODO certs should return err
	secret := certs.GetSecretContent(secretFile)
	if secret == nil {
		log.Println("Failed to make connector, missing connection-token content")
		return nil
	}

	existing, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve transport container (need init?): ", err.Error())
	}

	mode := qdr.GetTransportMode(existing)

	if options.Name == "" {
		options.Name = generateConnectorName(types.ConnPath)
	}
	connPath := types.ConnPath + options.Name

	if err := os.Mkdir(connPath, 0755); err != nil {
		log.Println("Failed to create skupper connector directory: ", err.Error())
	}
	for k, v := range secret {
		if err := ioutil.WriteFile(connPath+"/"+k, v, 0755); err != nil {
			log.Println("Failed to write connector certificate file: ", err.Error())
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
	log.Printf("Skupper configured to connect to %s:%s (name=%s)\n", connector.Host, connector.Port, connector.Name)

	err = docker.RestartTransportContainer(cli.DockerInterface)
	if err != nil {
		log.Println("Failed to re-start transport container: ", err.Error())
	}

	err = docker.RestartControllerContainer(cli.DockerInterface)
	if err != nil {
	    log.Println("Failed to re-start controller container", err.Error())
	}

	return err
}
