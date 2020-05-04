package client

import (
	"fmt"
	//	"io/ioutil"
	"log"
	//	"os"
	//	"regexp"
	//	"strconv"
	"time"

	//	"github.com/skupperproject/skupper-cli/pkg/certs"
	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func (cli *VanClient) VanRouterInspect() (*types.VanRouterInspectResponse, error) {
	vir := &types.VanRouterInspectResponse{}

	transport, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve transport container (need init?): ", err.Error())
		return vir, err
	}
	vir.TransportVersion = fmt.Sprintf("%s (%s)", transport.Config.Image, transport.Image[:19])

	controller, err := docker.InspectContainer("skupper-proxy-controller", cli.DockerInterface)
	if err != nil {
		log.Println("Failed to retrieve controller container (need init?): ", err.Error())
		return vir, err
	}
	vir.ControllerVersion = fmt.Sprintf("%s (%s)", controller.Config.Image, controller.Image[:19])

	vir.Status.Mode = string(qdr.GetTransportMode(transport))
	connected, err := qdr.GetConnectedSites(cli.DockerInterface)
	for i := 0; i < 5 && err != nil; i++ {
		time.Sleep(500 * time.Millisecond)
		connected, err = qdr.GetConnectedSites(cli.DockerInterface)
	}
	if err != nil {
		return vir, err
	}
	vir.Status.ConnectedSites = connected

	vsis, err := cli.VanServiceInterfaceList()
	if err != nil {
		vir.ExposedServices = 0
	} else {
		vir.ExposedServices = len(vsis)
	}

	return vir, err
}
