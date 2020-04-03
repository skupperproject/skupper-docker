package client

import (
	"io/ioutil"
	"log"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func getConnector(name string, mode types.TransportMode) (*types.Connector, error) {
	var role types.ConnectorRole
	var suffix string

	if mode == types.TransportModeEdge {
		role = types.ConnectorRoleEdge
		suffix = "/edge-"
	} else {
		role = types.ConnectorRoleInterRouter
		suffix = "/inter-router-"
	}
	host, err := ioutil.ReadFile(types.ConnPath + name + suffix + "host")
	if err != nil {
		log.Fatal("Could not retrieve connection-token files: ", err.Error())
		return &types.Connector{}, err
	}
	port, err := ioutil.ReadFile(types.ConnPath + name + suffix + "port")
	if err != nil {
		log.Fatal("Could not retrieve connection-token files: ", err.Error())
		return &types.Connector{}, err
	}
	connector := &types.Connector{
		Name: name,
		Host: string(host),
		Port: string(port),
		Role: string(role),
	}

	return connector, nil
}

func (cli *VanClient) VanConnectorInspect(name string) (*types.VanConnectorInspectResponse, error) {
	vci := &types.VanConnectorInspectResponse{}

	current, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		// TODO: is not found versus error
		log.Println("Unable to retrieve transport container (need init?)", err.Error())
		return vci, err
	}

	mode := qdr.GetTransportMode(current)
	connector, err := getConnector(name, mode)
	if err != nil {
		log.Println("Unable to get connector", err.Error())
	} else {
		vci.Connector = connector
	}

	connections, err := qdr.GetConnections(cli.DockerInterface)

	if err == nil {
		connection := qdr.GetInterRouterOrEdgeConnection(vci.Connector.Host+":"+vci.Connector.Port, connections)
		if connection == nil || !connection.Active {
			vci.Connected = false
		} else {
			vci.Connected = true
		}
	}
	return vci, nil
}
