package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockermounttypes "github.com/docker/docker/api/types/mount"
	dockernetworktypes "github.com/docker/docker/api/types/network"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker/libdocker"
	skupperutils "github.com/skupperproject/skupper/pkg/utils"
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

func WaitContainer(name string, dd libdocker.Interface) error {
	return dd.WaitContainer(name, 10*time.Second)
}

// TODO: should skupper containers be here or in another package?

func getLabels(service types.ServiceInterface, isLocal bool) map[string]string {
	target := ""
	targetType := "container"
	if isLocal {
		targetType = "host"
	}

	lastApplied, err := json.Marshal(service)
	if err != nil {
		log.Println("Failed to created json for proxy labels: ", err.Error())
	}

	return map[string]string{
		"skupper.io/application":      "skupper-proxy",
		"skupper.io/address":          service.Address,
		"skupper.io/target":           target,
		"skupper.io/targetType":       targetType,
		"skupper.io/component":        "proxy",
		"internal.skupper.io/type":    "proxy",
		"internal.skupper.io/service": service.Address,
		"skupper.io/origin":           service.Origin,
		"skupper.io/last-applied":     string(lastApplied),
	}
}

func getProxyContainerCreateConfig(service types.ServiceInterface, config string, osType string) *dockertypes.ContainerCreateConfig {
	var imageName string
	if os.Getenv("QDROUTERD_IMAGE") != "" {
		imageName = os.Getenv("QDROUTERD_IMAGE")
	} else {
		imageName = types.DefaultTransportImage
	}

	labels := getLabels(service, true)
	envVars := []string{}
	envVars = append(envVars, os.Getenv("SKUPPER_TMPDIR"))
	envVars = append(envVars, "QDROUTERD_CONF="+config)
	envVars = append(envVars, "QDROUTERD_CONF_TYPE=json")
	envVars = append(envVars, "NAMESPACE=skupper")
	if os.Getenv("PN_TRACE_FRM") != "" {
		envVars = append(envVars, "PN_TRACE_FRM=1")
	}

	var host string
	if os.Getenv("SKUPPER_HOST") != "" {
		host = os.Getenv("SKUPPER_HOST")
	} else {
		// magic address
		host = "172.17.0.1"
	}

	extraHosts := []string{}
	for _, t := range service.Targets {
		if t.Selector == "internal.skupper.io/host-service" {
			parts := strings.SplitN(t.Name, ":", 2)
			if len(parts) == 2 {
				if osType == "linux" && parts[1] == "host-gateway" {
					parts[1] = host
				}
				extraHosts = append(extraHosts, parts[0]+":"+parts[1])
			}
		}
	}

	containerCfg := &dockercontainer.Config{
		Hostname: service.Address,
		Image:    imageName,
		Env:      envVars,
		Labels:   labels,
	}
	hostCfg := &dockercontainer.HostConfig{
		Mounts: []dockermounttypes.Mount{
			{
				Type:   dockermounttypes.TypeBind,
				Source: types.GetSkupperPath(types.CertsPath) + "/" + "skupper-internal",
				Target: "/etc/qpid-dispatch-certs/skupper-internal/",
			},
		},
		ExtraHosts: extraHosts,
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

func NewProxyContainer(svcDef types.ServiceInterface, config string, dd libdocker.Interface) (*dockertypes.ContainerCreateConfig, error) {
	version, err := dd.ServerVersion()
	if err != nil {
		return nil, err
	}
	opts := getProxyContainerCreateConfig(svcDef, config, version.Os)

	_, err = dd.CreateContainer(*opts)
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

	containerCfg := &dockercontainer.Config{
		Hostname: current.Config.Hostname,
		Image:    current.Config.Image,
		Healthcheck: &dockercontainer.HealthConfig{
			Test:        []string{"curl --fail -s http://localhost:9090/healthz || exit 1"},
			StartPeriod: (time.Duration(60) * time.Second),
		},
		Labels:       current.Config.Labels,
		ExposedPorts: current.Config.ExposedPorts,
		Env:          current.Config.Env,
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

func WaitForContainerStatus(name string, status string, timeout time.Duration, interval time.Duration, dd libdocker.Interface) (*dockertypes.ContainerJSON, error) {
	var container *dockertypes.ContainerJSON
	var err error

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	err = skupperutils.RetryWithContext(ctx, interval, func() (bool, error) {
		container, err = InspectContainer(name, dd)
		if err != nil {
			return false, nil
		}
		return container.State.Status == status, nil
	})
	return container, err
}

func GetImageVersion(image string, dd libdocker.Interface) (string, error) {
	iibd, err := dd.InspectImageByID(image)
	if err != nil {
		return "", err
	}

	digest := iibd.RepoDigests[0]
	parts := strings.Split(digest, "@")
	if len(parts) > 1 && len(parts[1]) >= 19 {
		return fmt.Sprintf("%s (%s)", image, parts[1][:19]), nil
	} else {
		return fmt.Sprintf("%s", digest), nil
	}
}
