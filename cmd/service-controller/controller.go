package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	amqp "github.com/interconnectedcloud/go-amqp"

	dockertypes "github.com/docker/docker/api/types"
	dockerfilters "github.com/docker/docker/api/types/filters"

	"github.com/fsnotify/fsnotify"
	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/client"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

type Controller struct {
	origin    string
	vanClient *client.VanClient

	// controller loop state
	bindings map[string]*ServiceBindings

	// service_sync statue
	tlsConfig       *tls.Config
	amqpClient      *amqp.Client
	amqpSession     *amqp.Session
	byOrigin        map[string]map[string]types.ServiceInterface
	localServices   map[string]types.ServiceInterface
	byName          map[string]types.ServiceInterface
	desiredServices map[string]types.ServiceInterface
	heardFrom       map[string]time.Time
}

func equivalentProxyConfig(desired types.ServiceInterface, env []string) bool {
	envVar := docker.FindEnvVar(env, "SKUPPER_PROXY_CONFIG")
	encodedDesired, _ := json.Marshal(desired)
	return string(encodedDesired) == envVar
}

func NewController(cli *client.VanClient, origin string, tlsConfig *tls.Config) (*Controller, error) {
	controller := &Controller{
		vanClient: cli,
		origin:    origin,
		tlsConfig: tlsConfig,
	}

	// Organize service definitions
	controller.bindings = make(map[string]*ServiceBindings)
	controller.byOrigin = make(map[string]map[string]types.ServiceInterface)
	controller.localServices = make(map[string]types.ServiceInterface)
	controller.byName = make(map[string]types.ServiceInterface)
	controller.desiredServices = make(map[string]types.ServiceInterface)
	controller.heardFrom = make(map[string]time.Time)

	// could setup watchers here

	return controller, nil
}

func (c *Controller) Run(stopCh <-chan struct{}) error {
	log.Println("Starting the Skupper controller")

	var imageName string
	if os.Getenv("QDROUTERD_IMAGE") != "" {
		imageName = os.Getenv("QDROUTERD_IMAGE")
	} else {
		imageName = types.DefaultTransportImage
	}

	log.Println("Pulling proxy image")
	err := c.vanClient.DockerInterface.PullImage(imageName, dockertypes.AuthConfig{}, dockertypes.ImagePullOptions{})
	if err != nil {
		log.Fatal("Failed to pull proxy image: ", err.Error())
	}

	log.Println("Starting workers")
	go c.runServiceSync() // receives peer updates
	go c.runServiceDefsWatcher()

	log.Println("Started workers")
	<-stopCh
	log.Println("Shutting down workers")

	return nil
}

func updateSkupperServices(changed []types.ServiceInterface, deleted []string, origin string) error {
	if len(changed) == 0 && len(deleted) == 0 {
		return nil
	}

	current := make(map[string]types.ServiceInterface)
	file, err := ioutil.ReadFile("/etc/messaging/services/skupper-services")

	if err != nil {
		return fmt.Errorf("Failed to retrieve skupper service definitions: %w", err)
	}
	err = json.Unmarshal([]byte(file), &current)
	if err != nil {
		return fmt.Errorf("Failed to decode json for service definitions: %w", err)
	}

	for _, def := range changed {
		current[def.Address] = def
	}

	for _, name := range deleted {
		delete(current, name)
	}

	encoded, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("Failed to encode json for service interface: %w", err)
	}

	err = ioutil.WriteFile("/etc/messaging/services/skupper-services", encoded, 0755)
	if err != nil {
		return fmt.Errorf("Failed to write service file: %w", err)
	}
	return nil
}

func getServiceDefinitions() (map[string]types.ServiceInterface, error) {
	svcDefs := make(map[string]types.ServiceInterface)
	file, err := ioutil.ReadFile("/etc/messaging/services/skupper-services")
	if err != nil {
		return svcDefs, fmt.Errorf("Failed to retrieve skupper service definitions: %w", err)
	}
	err = json.Unmarshal([]byte(file), &svcDefs)
	if err != nil {
		return svcDefs, fmt.Errorf("Failed to decode json for service definitions: %w", err)
	}
	return svcDefs, nil
}

func (c *Controller) ensureProxyFor(bindings *ServiceBindings) error {
	proxies := c.getProxies()
	_, exists := proxies[bindings.address]
	serviceInterface := asServiceInterface(bindings)

	if bindings.origin == "" {
		attached := make(map[string]dockertypes.EndpointResource)
		sn, err := docker.InspectNetwork(types.TransportNetworkName, c.vanClient.DockerInterface)
		if err != nil {
			return fmt.Errorf("Unable to retrieve skupper-network: %w", err)
		}
		for _, c := range sn.Containers {
			attached[c.Name] = c
		}

		for _, t := range bindings.targets {
			if t.selector == "internal.skupper.io/container" {
				if _, ok := attached[t.name]; !ok {
					fmt.Println("Attaching container to skupper network: ", t.service)
					err := docker.ConnectContainerToNetwork(types.TransportNetworkName, t.name, c.vanClient.DockerInterface)
					if err != nil {
						log.Println("Failed to attach target container to skupper network: ", err.Error())
					}
				}
			}
		}
	}

	config, _ := qdr.GetRouterConfigForProxy(serviceInterface, c.origin)

	if !exists {
		log.Println("Deploying proxy: ", serviceInterface.Address)
		proxyContainer, err := docker.NewProxyContainer(serviceInterface, config, c.vanClient.DockerInterface)
		if err != nil {
			return fmt.Errorf("Failed to create proxy container: %w", err)
		}
		err = docker.StartContainer(proxyContainer.Name, c.vanClient.DockerInterface)
		if err != nil {
			return fmt.Errorf("Failed to start proxy container: %w", err)
		}
	} else {
		proxyContainer, err := docker.InspectContainer(serviceInterface.Address, c.vanClient.DockerInterface)
		if err != nil {
			return fmt.Errorf("Failed to retrieve current proxy container: %w", err)
		}
		actualConfig := docker.FindEnvVar(proxyContainer.Config.Env, "QDROUTERD_CONF")
		if actualConfig == "" || actualConfig != config {
			log.Println("Updating proxy config for: ", serviceInterface.Address)
			err := c.deleteProxy(serviceInterface.Address)
			if err != nil {
				return fmt.Errorf("Failed to delete proxy container: %w", err)
			}
			newProxyContainer, err := docker.NewProxyContainer(serviceInterface, config, c.vanClient.DockerInterface)
			if err != nil {
				return fmt.Errorf("Failed to re-create proxy container: %w", err)
			}
			err = docker.StartContainer(newProxyContainer.Name, c.vanClient.DockerInterface)
			if err != nil {
				return fmt.Errorf("Failed to start proxy container: %w", err)
			}
		}
	}
	return nil

}

func (c *Controller) deleteProxy(name string) error {
	err := docker.StopContainer(name, c.vanClient.DockerInterface)
	if err != nil {
		return err
	}
	err = docker.RemoveContainer(name, c.vanClient.DockerInterface)
	return err
}

func (c *Controller) updateProxies() {
	for _, v := range c.bindings {
		err := c.ensureProxyFor(v)
		if err != nil {
			log.Println("Unable to ensure proxy container: ", err.Error())
		}
	}
	proxies := c.getProxies()
	for _, v := range proxies {
		proxyContainerName := strings.TrimPrefix(v.Names[0], "/")
		def, ok := c.bindings[proxyContainerName]
		if !ok || def == nil {
			c.deleteProxy(proxyContainerName)
		}
	}
}

func (c *Controller) getProxies() map[string]dockertypes.Container {
	proxies := make(map[string]dockertypes.Container)

	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/application")
	opts := dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	}
	containers, err := docker.ListContainers(opts, c.vanClient.DockerInterface)
	if err == nil {
		for _, container := range containers {
			proxyName := strings.TrimPrefix(container.Names[0], "/")
			proxies[proxyName] = container
		}
	}
	return proxies
}

func (c *Controller) processServiceDefs() {
	svcDefs, err := getServiceDefinitions()
	if err != nil {
		log.Println("Failed to retrieve skupper service definitions: ", err.Error())
		return
	}
	c.serviceSyncDefinitionsUpdated(svcDefs)
	if len(svcDefs) > 0 {
		for _, v := range svcDefs {
			c.updateServiceBindings(v)
		}
		for k, _ := range c.bindings {
			_, ok := svcDefs[k]
			if !ok {
				delete(c.bindings, k)
			}
		}
	} else if len(c.bindings) > 0 {
		for k, _ := range c.bindings {
			delete(c.bindings, k)
		}
	}
	c.updateProxies()
}

func (c *Controller) runServiceDefsWatcher() {
	var watcher *fsnotify.Watcher

	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	err := watcher.Add("/etc/messaging/services/skupper-services")
	if err != nil {
		log.Println("Could not add directory watcher", err.Error())
		return
	}

	c.processServiceDefs()

	for origin, _ := range c.byOrigin {
		if origin != c.origin {
			c.heardFrom[origin] = time.Now()
		}
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				c.processServiceDefs()
			}
		}
	}

}
