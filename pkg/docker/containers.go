package docker

import (
	//    "os"
	"io/ioutil"
	"log"
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

func getControllerContainerCreateConfig(van *types.VanRouterSpec) *dockertypes.ContainerCreateConfig {
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
			Cmd:      []string{"/go/src/app/controller"},
			Env:      van.Controller.EnvVar,
			Labels:   van.Controller.Labels,
		},
		HostConfig: &dockercontainer.HostConfig{
			Mounts:     mounts,
			Privileged: true,
		},
		NetworkingConfig: &dockernetworktypes.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
				"skupper-network": {},
			},
		},
	}

	return opts
}

func getTransportContainerCreateConfig(van *types.VanRouterSpec) *dockertypes.ContainerCreateConfig {
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
				StartPeriod: time.Duration(60),
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
				"skupper-network": {},
			},
		},
	}

	return opts
}

// TODO: unify the two news
func NewControllerContainer(van *types.VanRouterSpec, dd libdocker.Interface) (*dockertypes.ContainerCreateConfig, error) {
	opts := getControllerContainerCreateConfig(van)

	// TODO: where should create and start be, here or in up a
	_, err := dd.CreateContainer(*opts)
	if err != nil {
		return nil, err
	} else {
		return opts, nil
	}

}

func RestartControllerContainer(dd libdocker.Interface) error {
	current, err := InspectContainer("skupper-proxy-controller", dd)
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

	containerCfg := &dockercontainer.Config{
		Hostname:     current.Config.Hostname,
		Image:        current.Config.Image,
		Cmd:          current.Config.Cmd,
		Labels:       current.Config.Labels,
		ExposedPorts: current.Config.ExposedPorts,
		Env:          current.Config.Env,
	}

	// remove current and create new container
	err = StopContainer("skupper-proxy-controller", dd)
	if err != nil {
		log.Println("Failed to stop controller container", err.Error())
	}

	err = RemoveContainer("skupper-proxy-controller", dd)
	if err != nil {
		log.Println("Failed to remove controller container", err.Error())
	}

	opts := &dockertypes.ContainerCreateConfig{
		Name:       "skupper-proxy-controller",
		Config:     containerCfg,
		HostConfig: hostCfg,
		NetworkingConfig: &dockernetworktypes.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
				"skupper-network": {},
			},
		},
	}

	_, err = CreateContainer(opts, dd)
	if err != nil {
		log.Println("Failed to re-create controller container", err.Error())
	}

	err = StartContainer("skupper-proxy-controller", dd)
	if err != nil {
		log.Println("Failed to re-start controller container", err.Error())
	}

	return nil
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
			StartPeriod: time.Duration(60),
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
				"skupper-network": {},
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

func NewTransportContainer(van *types.VanRouterSpec, dd libdocker.Interface) (*dockertypes.ContainerCreateConfig, error) {

	opts := getTransportContainerCreateConfig(van)

	// TODO: where should create and start be, here or in up a
	_, err := dd.CreateContainer(*opts)
	if err != nil {
		return nil, err
	} else {
		return opts, nil
	}

	return nil, nil
}
