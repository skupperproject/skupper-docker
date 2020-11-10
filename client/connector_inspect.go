package client

import (
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func (cli *VanClient) ConnectorInspect(name string) (*types.ConnectorInspectResponse, error) {
	vci := &types.ConnectorInspectResponse{}
	var role types.ConnectorRole
	var suffix string

	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		// TODO: is not found versus error
		return vci, fmt.Errorf("Unable to retrieve transport container (need init?): %w", err)
	}

	current, err := qdr.GetRouterConfigFromFile(types.ConfigPath + "/qdrouterd.json")
	if err != nil {
		return vci, fmt.Errorf("Failed to retrieve router config: %w", err)
	}

	if current.IsEdge() {
		role = types.ConnectorRoleEdge
		suffix = "/edge-"
	} else {
		role = types.ConnectorRoleInterRouter
		suffix = "/inter-router-"
	}

	host, err := ioutil.ReadFile(types.ConnPath + name + suffix + "host")
	if err != nil {
		return vci, fmt.Errorf("Could not retrieve connection-token files: %w", err)
	}
	port, err := ioutil.ReadFile(types.ConnPath + name + suffix + "port")
	if err != nil {
		return vci, fmt.Errorf("Could not retrieve connection-token files: %w", err)
	}
	vci.Connector = &types.Connector{
		Name: name,
		Host: string(host),
		Port: string(port),
		Role: string(role),
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
