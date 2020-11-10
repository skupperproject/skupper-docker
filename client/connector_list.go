package client

import (
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func (cli *VanClient) ConnectorList() ([]*types.Connector, error) {
	var connectors []*types.Connector
	// verify that the transport is interior mode
	_, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		return connectors, fmt.Errorf("Unable to retrieve transport container (need init?): %w", err)
	}

	current, err := qdr.GetRouterConfigFromFile(types.ConfigPath + "/qdrouterd.json")
	if err != nil {
		return connectors, fmt.Errorf("Failed to retrieve router config: %w", err)
	}

	files, err := ioutil.ReadDir(types.ConnPath)
	if err != nil {
		return connectors, fmt.Errorf("Failed to read connector definitions: %w", err)
	}

	var role types.ConnectorRole
	var host []byte
	var port []byte
	var suffix string
	if current.IsEdge() {
		role = types.ConnectorRoleEdge
		suffix = "/edge-"
	} else {
		role = types.ConnectorRoleInterRouter
		suffix = "/inter-router-"
	}

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
	return connectors, nil
}
