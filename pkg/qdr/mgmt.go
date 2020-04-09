/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package qdr

import (
	"encoding/json"
	"fmt"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker/libdocker"
)

type RouterNode struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	NextHop string `json:"nextHop"`
}

type ConnectedSites struct {
	Direct   int
	Indirect int
	Total    int
}

type Connection struct {
	Container  string `json:"container"`
	OperStatus string `json:"operStatus"`
	Host       string `json:"host"`
	Role       string `json:"role"`
	Active     bool   `json:"active"`
	Dir        string `json:"dir"`
}

func getQuery(typename string) []string {
	return []string{
		"qdmanage",
		"query",
		"--type",
		typename,
	}
}

func GetConnectedSites(dd libdocker.Interface) (types.TransportConnectedSites, error) {
	result := types.TransportConnectedSites{}
	nodes, err := GetNodes(dd)
	if err == nil {
		for _, n := range nodes {
			if n.NextHop == "" {
				result.Direct++
				result.Total++
			} else if n.NextHop != "(self)" {
				result.Indirect++
				result.Total++
			}
		}
	}
	return result, err
}

func GetNodes(dd libdocker.Interface) ([]RouterNode, error) {
	command := getQuery("node")
	execResult, err := routerExec(command, dd)
	if err != nil {
		return nil, err
	} else {
		results := []RouterNode{}
		err = json.Unmarshal(execResult.outBuffer.Bytes(), &results)
		if err != nil {
			fmt.Println("Failed to parse JSON: ", err.Error(), execResult.outBuffer.String())
			return nil, err
		} else {
			return results, nil
		}
	}
}

func GetInterRouterOrEdgeConnection(host string, connections []Connection) *Connection {
	for _, c := range connections {
		if (c.Role == "inter-router" || c.Role == "edge") && c.Host == host {
			return &c
		}
	}
	return nil
}

func GetConnections(dd libdocker.Interface) ([]Connection, error) {
	command := getQuery("connection")
	execResult, err := routerExec(command, dd)
	if err != nil {
		return nil, err
	} else {
		results := []Connection{}
		err = json.Unmarshal(execResult.outBuffer.Bytes(), &results)
		if err != nil {
			fmt.Println("Failed to parse JSON: ", err.Error(), execResult.outBuffer.String())
			return nil, err
		} else {
			return results, nil
		}
	}
}

func routerExec(command []string, dd libdocker.Interface) (ExecResult, error) {

	current, err := dd.InspectContainer("skupper-router")
	if err != nil {
		fmt.Println("Error retrieving skupper router container: ", err.Error())
		return ExecResult{}, err
	}

	result, err := Exec(dd, current.ID, command)
	if err != nil {
		fmt.Println("Error exec into skupper router container: ", err.Error())
		return ExecResult{}, err
	}
	return result, nil
}
