package client

import (
	"fmt"
	"io/ioutil"

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
		return &types.Connector{}, fmt.Errorf("Could not retrieve connection-token files: %w", err)
	}
	port, err := ioutil.ReadFile(types.ConnPath + name + suffix + "port")
	if err != nil {
		return &types.Connector{}, fmt.Errorf("Could not retrieve connection-token files: %w", err)
	}
	connector := &types.Connector{
		Name: name,
		Host: string(host),
		Port: string(port),
		Role: string(role),
	}

	return connector, nil
}

func (cli *VanClient) ConnectorInspect(name string) (*types.ConnectorInspectResponse, error) {
	vci := &types.ConnectorInspectResponse{}

	current, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		// TODO: is not found versus error
		return vci, fmt.Errorf("Unable to retrieve transport container (need init?): %w", err)
	}

	mode := qdr.GetTransportMode(current)
	connector, err := getConnector(name, mode)
	if err != nil {
		return vci, fmt.Errorf("Unable to get connector: %w", err)
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
		return vci, nil
	} else {
		return vci, fmt.Errorf("Unable to get connections from transport: %w", err)
	}
}
