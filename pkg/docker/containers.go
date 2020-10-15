package docker

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockermounttypes "github.com/docker/docker/api/types/mount"
	dockernetworktypes "github.com/docker/docker/api/types/network"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker/libdocker"
	"github.com/skupperproject/skupper-docker/pkg/utils/configs"
)

func CreateContainer(opts *dockertypes.ContainerCreateConfig, dd libdocker.Interface) (*dockercontainer.ContainerCreateCreatedBody, error) {
	cccb, err := dd.CreateContainer(*opts)
	return cccb, err
}

func InspectContainer(name string, dd libdocker.Interface) (*dockertypes.ContainerJSON, error) {
	return dd.InspectContainer(name)
}

func ListContainers(opts dockertypes.ContainerListOptions, dd libdocker.Interface) ([]dockertypes.Container, error) {
	return dd.ListContainers(opts)
}

func ConnectContainerToNetwork(network string, name string, dd libdocker.Interface) error {
	return dd.ConnectContainerToNetwork(network, name)
}

func RestartContainer(name string, dd libdocker.Interface) error {
	return dd.RestartContainer(name, 10*time.Second)
}

func RemoveContainer(name string, dd libdocker.Interface) error {
	return dd.RemoveContainer(name, dockertypes.ContainerRemoveOptions{})
}

func StopContainer(name string, dd libdocker.Interface) error {
	return dd.StopContainer(name, 10*time.Second)
}

func StartContainer(name string, dd libdocker.Interface) error {
	return dd.StartContainer(name)
}

// TODO: should skupper containers be here or in another package?

func getLabels(service types.ServiceInterface, isLocal bool) map[string]string {
	target := ""
	targetType := "container"
	if isLocal {
		target = service.Targets[0].Name
		if service.Targets[0].Selector == "internal.skupper.io/hostservice" {
			targetType = "host"
		}
	}

	lastApplied, err := json.Marshal(service)
	if err != nil {
		log.Println("Failed to created json for proxy labels: ", err.Error())
	}

	return map[string]string{
		"skupper.io/application":  "skupper-proxy",
		"skupper.io/address":      service.Address,
		"skupper.io/target":       target,
		"skupper.io/targetType":   targetType,
		"skupper.io/component":    "proxy",
		"skupper.io/origin":       service.Origin,
		"skupper.io/last-applied": string(lastApplied),
	}
}

func getProxyContainerCreateConfig(service types.ServiceInterface, isLocal bool) *dockertypes.ContainerCreateConfig {
	var imageName string
	if os.Getenv("PROXY_IMAGE") != "" {
		imageName = os.Getenv("PROXY_IMAGE")
	} else {
		imageName = types.DefaultProxyImage
	}

	labels := getLabels(service, isLocal)
	bridges := []string{}
	env := []string{}

	bridges = append(bridges, service.Protocol+":"+strconv.Itoa(int(service.Port))+"=>amqp:"+service.Address)
	if isLocal {
		bridges = append(bridges, "amqp:"+service.Address+"=>"+service.Protocol+":"+strconv.Itoa(int(service.Targets[0].TargetPort)))
		env = append(env, "ICPROXY_BRIDGE_HOST="+service.Targets[0].Name)
	}
	bridgeCfg := strings.Join(bridges, ",")

	containerCfg := &dockercontainer.Config{
		Hostname: service.Address,
		Image:    imageName,
		Cmd: []string{
			"node",
			"/opt/app-root/bin/simple.js",
			bridgeCfg},
		Env:    env,
		Labels: labels,
	}
	hostCfg := &dockercontainer.HostConfig{
		Mounts: []dockermounttypes.Mount{
			{
				Type:   dockermounttypes.TypeBind,
				Source: types.CertPath + "skupper",
				Target: "/etc/messaging",
			},
		},
		Privileged: true,
	}
	networkCfg := &dockernetworktypes.NetworkingConfig{
		EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
			types.TransportNetworkName: {},
		},
	}

	opts := &dockertypes.ContainerCreateConfig{
		Name:             service.Address,
		Config:           containerCfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkCfg,
	}

	return opts
}

func NewProxyContainer(svcDef types.ServiceInterface, isLocal bool, dd libdocker.Interface) (*dockertypes.ContainerCreateConfig, error) {
	opts := getProxyContainerCreateConfig(svcDef, isLocal)

	// TODO: where should create and start be, here or in up a
	_, err := dd.CreateContainer(*opts)
	if err != nil {
		return nil, err
	} else {
		return opts, nil
	}

}

func RestartControllerContainer(dd libdocker.Interface) error {
	current, err := InspectContainer(types.ControllerDeploymentName, dd)
	if err != nil {
		return err
	}

	mounts := []dockermounttypes.Mount{}
	for _, v := range current.Mounts {
		mounts = append(mounts, dockermounttypes.Mount{
			Type:   v.Type,
			Source: v.Source,
			Target: v.Destination,
		})
	}
	hostCfg := &dockercontainer.HostConfig{
		Mounts:     mounts,
		Privileged: true,
	}

	newEnv := SetEnvVar(current.Config.Env, "SKUPPER_PROXY_CONTROLLER_RESTART", "true")

	containerCfg := &dockercontainer.Config{
		Hostname:     current.Config.Hostname,
		Image:        current.Config.Image,
		Cmd:          current.Config.Cmd,
		Labels:       current.Config.Labels,
		ExposedPorts: current.Config.ExposedPorts,
		Env:          newEnv,
	}

	// remove current and create new container
	err = StopContainer(types.ControllerDeploymentName, dd)
	if err != nil {
		log.Println("Failed to stop controller container", err.Error())
	}

	err = RemoveContainer(types.ControllerDeploymentName, dd)
	if err != nil {
		log.Println("Failed to remove controller container", err.Error())
	}

	opts := &dockertypes.ContainerCreateConfig{
		Name:       types.ControllerDeploymentName,
		Config:     containerCfg,
		HostConfig: hostCfg,
		NetworkingConfig: &dockernetworktypes.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
				types.TransportNetworkName: {},
			},
		},
	}

	_, err = CreateContainer(opts, dd)
	if err != nil {
		log.Println("Failed to re-create controller container", err.Error())
	}

	err = StartContainer(types.ControllerDeploymentName, dd)
	if err != nil {
		log.Println("Failed to re-start controller container", err.Error())
	}

	return nil
}

func getControllerContainerCreateConfig(van *types.RouterSpec) *dockertypes.ContainerCreateConfig {
	mounts := []dockermounttypes.Mount{}
	for source, target := range van.Controller.Mounts {
		mounts = append(mounts, dockermounttypes.Mount{
			Type:   dockermounttypes.TypeBind,
			Source: source,
			Target: target,
		})
	}

	opts := &dockertypes.ContainerCreateConfig{
		Name: types.ControllerDeploymentName,
		Config: &dockercontainer.Config{
			Hostname: types.ControllerDeploymentName,
			Image:    van.Controller.Image,
			Cmd:      []string{"/app/controller"},
			Env:      van.Controller.EnvVar,
			Labels:   van.Controller.Labels,
		},
		HostConfig: &dockercontainer.HostConfig{
			Mounts:     mounts,
			Privileged: true,
		},
		NetworkingConfig: &dockernetworktypes.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
				types.TransportNetworkName: {},
			},
		},
	}

	return opts
}

// TODO: unify the two news
func NewControllerContainer(van *types.RouterSpec, dd libdocker.Interface) (*dockertypes.ContainerCreateConfig, error) {
	opts := getControllerContainerCreateConfig(van)

	// TODO: where should create and start be, here or in up a
	_, err := dd.CreateContainer(*opts)
	if err != nil {
		return nil, err
	} else {
		return opts, nil
	}

}

func RestartTransportContainer(dd libdocker.Interface) error {
	current, err := InspectContainer("skupper-router", dd)
	if err != nil {
		return err
	}

	mounts := []dockermounttypes.Mount{}
	for _, v := range current.Mounts {
		mounts = append(mounts, dockermounttypes.Mount{
			Type:   v.Type,
			Source: v.Source,
			Target: v.Destination,
		})
	}
	hostCfg := &dockercontainer.HostConfig{
		Mounts:     mounts,
		Privileged: true,
	}

	// grab the env and add connectors to it, splice off current ones
	currentEnv := current.Config.Env
	pattern := "## Connectors: ##"
	transportConf := FindEnvVar(currentEnv, types.TransportEnvConfig)
	updated := strings.Split(transportConf, pattern)[0] + pattern

	files, err := ioutil.ReadDir(types.ConnPath)
	for _, f := range files {
		connName := f.Name()
		hostString, _ := ioutil.ReadFile(types.ConnPath + connName + "/inter-router-host")
		portString, _ := ioutil.ReadFile(types.ConnPath + connName + "/inter-router-port")
		connector := types.Connector{
			Name: connName,
			Host: string(hostString),
			Port: string(portString),
			Role: string(types.ConnectorRoleInterRouter),
		}
		updated += configs.ConnectorConfig(&connector)
	}

	newEnv := SetEnvVar(currentEnv, types.TransportEnvConfig, updated)

	containerCfg := &dockercontainer.Config{
		Hostname: current.Config.Hostname,
		Image:    current.Config.Image,
		Healthcheck: &dockercontainer.HealthConfig{
			Test:        []string{"curl --fail -s http://localhost:9090/healthz || exit 1"},
			StartPeriod: (time.Duration(60) * time.Second),
		},
		Labels:       current.Config.Labels,
		ExposedPorts: current.Config.ExposedPorts,
		Env:          newEnv,
	}

	// remove current and create new container
	err = StopContainer("skupper-router", dd)
	if err != nil {
		log.Println("Failed to stop transport container", err.Error())
	}

	err = RemoveContainer("skupper-router", dd)
	if err != nil {
		log.Println("Failed to remove transport container", err.Error())
	}

	opts := &dockertypes.ContainerCreateConfig{
		Name:       "skupper-router",
		Config:     containerCfg,
		HostConfig: hostCfg,
		NetworkingConfig: &dockernetworktypes.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
				types.TransportNetworkName: {},
			},
		},
	}

	_, err = CreateContainer(opts, dd)
	if err != nil {
		log.Println("Failed to re-create transport container", err.Error())
	}

	err = StartContainer("skupper-router", dd)
	if err != nil {
		log.Println("Failed to re-start transport container", err.Error())
	}

	return nil
}

func getTransportContainerCreateConfig(van *types.RouterSpec) *dockertypes.ContainerCreateConfig {
	mounts := []dockermounttypes.Mount{}
	for source, target := range van.Transport.Mounts {
		mounts = append(mounts, dockermounttypes.Mount{
			Type:   dockermounttypes.TypeBind,
			Source: source,
			Target: target,
		})
	}

	opts := &dockertypes.ContainerCreateConfig{
		Name: types.TransportDeploymentName,
		Config: &dockercontainer.Config{
			Hostname: types.TransportDeploymentName,
			Image:    van.Transport.Image,
			Env:      van.Transport.EnvVar,
			Healthcheck: &dockercontainer.HealthConfig{
				Test:        []string{"curl --fail -s http://localhost:9090/healthz || exit 1"},
				StartPeriod: (time.Duration(60) * time.Second),
			},
			Labels:       van.Transport.Labels,
			ExposedPorts: van.Transport.Ports,
		},
		HostConfig: &dockercontainer.HostConfig{
			Mounts:     mounts,
			Privileged: true,
		},
		NetworkingConfig: &dockernetworktypes.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
				types.TransportNetworkName: {},
			},
		},
	}

	return opts
}

func NewTransportContainer(van *types.RouterSpec, dd libdocker.Interface) (*dockertypes.ContainerCreateConfig, error) {

	opts := getTransportContainerCreateConfig(van)

	// TODO: where should create and start be, here or in up a
	_, err := dd.CreateContainer(*opts)
	if err != nil {
		return nil, err
	}
	return opts, nil

}
