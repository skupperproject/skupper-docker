package client

import (
	"io/ioutil"
	"log"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func retrieveConnectors(mode types.TransportMode) ([]*types.Connector, error) {
	var connectors []*types.Connector
	files, err := ioutil.ReadDir(types.ConnPath)
	if err == nil {
		var role types.ConnectorRole
		var host []byte
		var port []byte
		var suffix string
		if mode == types.TransportModeEdge {
			role = types.ConnectorRoleEdge
			suffix = "/edge-"
		} else {
			role = types.ConnectorRoleInterRouter
			suffix = "/inter-router-"
		}
		// TODO handle err on read
		for _, f := range files {
			host, _ = ioutil.ReadFile(types.ConnPath + f.Name() + suffix + "host")
			port, _ = ioutil.ReadFile(types.ConnPath + f.Name() + suffix + "port")
			connectors = append(connectors, &types.Connector{
				Name: f.Name(),
				Host: string(host),
				Port: string(port),
				Role: string(role),
			})
		}
	} else {
		return connectors, err
	}
	return connectors, nil
}

func (cli *VanClient) VanConnectorList() ([]*types.Connector, error) {
	var connectors []*types.Connector
	// verify that the transport is interior mode
	current, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		// TODO: is not found versus error
		log.Println("Unable to retrieve transport container (need init?)", err.Error())
		return connectors, err
	}

	mode := qdr.GetTransportMode(current)
	return retrieveConnectors(mode)
}
