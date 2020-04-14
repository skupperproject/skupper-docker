package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	amqp "github.com/Azure/go-amqp"

	dockertypes "github.com/docker/docker/api/types"

	"github.com/fsnotify/fsnotify"
	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/client"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

type Controller struct {
	origin          string
	vanClient       *client.VanClient
	tlsConfig       *tls.Config
	amqpClient      *amqp.Client
	amqpSession     *amqp.Session
	byOrigin        map[string]map[string]types.ServiceInterface
	localServices   []types.ServiceInterface
	byName          map[string]types.ServiceInterface
	desiredServices map[string]types.ServiceInterface
	proxies         map[string]*dockertypes.ContainerJSON
}

func equivalentProxyConfig(desired types.ServiceInterface, env []string) bool {
	envVar := docker.FindEnvVar(env, "SKUPPER_PROXY_CONFIG")
	encodedDesired, _ := json.Marshal(desired)
	return string(encodedDesired) == envVar
}

func (c *Controller) printAllKeys() {
	depKeys := []string{}
	proxyKeys := []string{}

	for key, _ := range c.desiredServices {
		depKeys = append(depKeys, key)
	}
	for key, _ := range c.proxies {
		proxyKeys = append(proxyKeys, key)
	}

	log.Println("Desired services: ", depKeys)
	log.Println("Proxies: ", proxyKeys)
	log.Println("Local Services: ", c.localServices)

}

func NewController(cli *client.VanClient, origin string, tlsConfig *tls.Config) (*Controller, error) {
	controller := &Controller{
		vanClient: cli,
		origin:    origin,
		tlsConfig: tlsConfig,
	}

	// Organize service definitions
	controller.byOrigin = make(map[string]map[string]types.ServiceInterface)
	controller.byName = make(map[string]types.ServiceInterface)
	controller.desiredServices = make(map[string]types.ServiceInterface)
	controller.proxies = make(map[string]*dockertypes.ContainerJSON)

	return controller, nil
}

func (c *Controller) Run(stopCh <-chan struct{}) error {
	log.Println("Starting the Skupper controller")

	var imageName string
	if os.Getenv("PROXY_IMAGE") != "" {
		imageName = os.Getenv("PROXY_IMAGE")
	} else {
		imageName = "quay.io/skupper/icproxy-simple"
	}

	log.Println("Pull proxy image")
	err := c.vanClient.DockerInterface.PullImage(imageName, dockertypes.AuthConfig{}, dockertypes.ImagePullOptions{})
	if err != nil {
		log.Fatal("Failed to pull proxy image: ", err.Error())
	}

	// For the case of controller restart, need to setup the controller context that
	// we last left off at, we could check an env variable to do this?
	err = c.restoreControllerContext()
	if err != nil {
		log.Fatal("Failed to restore proxy context: ", err.Error())
	}

	log.Println("Starting workers")
	syncUpdate := make(chan *ServiceSyncUpdate)
	go c.runServiceSync(syncUpdate)
	go c.processUpdateWorkItem(syncUpdate)
	go c.processServiceWorkItem()

	log.Println("Started workers")
	<-stopCh
	log.Println("Shutting down workers")

	return nil
}

func getServices(name string) (map[string]types.ServiceInterface, error) {
	svcDefs := make(map[string]types.ServiceInterface)

	file, err := ioutil.ReadFile("/etc/messaging/services/" + name + "-skupper-services")
	if err != nil {
		return svcDefs, fmt.Errorf("Failed to retrieve skupper service definitions: %w", err)
	}
	err = json.Unmarshal([]byte(file), &svcDefs)
	if err != nil {
		return svcDefs, fmt.Errorf("Failed to decode json for service definitions: %w", err)
	}
	return svcDefs, nil
}

func updateServices(name string, svcDefs map[string]types.ServiceInterface) error {

	encoded, err := json.Marshal(svcDefs)
	if err != nil {
		return fmt.Errorf("Failed to encode json for service: %w", err)
	}
	err = ioutil.WriteFile("/etc/messaging/services/"+name+"-skupper-services", encoded, 0755)
	if err != nil {
		return fmt.Errorf("Failed to write service file: %w", err)
	}
	return nil
}

// TODO: return error
func (c *Controller) ensureProxyContainer(name string) {

	hostService := false
	proxyName := name
	proxy, proxyDefined := c.proxies[proxyName]
	serviceInterface := c.desiredServices[name]

	isLocal := serviceInterface.Origin == c.origin
	if isLocal {
		hostService = serviceInterface.Targets[0].Selector == "internal.skupper.io/hostservice"
	}

	// attach target container to the skupper network
	if !proxyDefined && isLocal && !hostService {
		err := docker.ConnectContainerToNetwork("skupper-network", serviceInterface.Targets[0].Name, c.vanClient.DockerInterface)
		if err != nil {
			log.Println("Failed to attach target container to skupper network: ", err.Error())
		}
	}

	if !proxyDefined {
		log.Printf("Need to create proxy for %s (%s)\n", serviceInterface.Address, proxyName)
		proxyContainer, err := docker.NewProxyContainer(serviceInterface, isLocal, c.vanClient.DockerInterface)
		if err != nil {
			log.Println("Failed to create proxy container config", err.Error())
			return
		}
		err = docker.StartContainer(proxyContainer.Name, c.vanClient.DockerInterface)
		if err != nil {
			log.Println("Failed to start proxy container", err.Error())
			return
		}
		proxyInspect, err := docker.InspectContainer(proxyContainer.Name, c.vanClient.DockerInterface)
		if err != nil {
			log.Println("Failed to inspect proxy container", err.Error())
			return
		}
		c.proxies[proxyName] = proxyInspect
	} else {
		if !equivalentProxyConfig(serviceInterface, proxy.Config.Env) {
			log.Println("TODO: Need to update proxy config for ", proxy.Name)
		} else {
			log.Println("TODO: Nothing to do here for proxy config", proxy.Name)
		}
	}
}

func (c *Controller) reconcile() error {
	log.Println("Reconciling...")

	// reconcile proxy deployment with desired services:
	for name, _ := range c.desiredServices {
		c.ensureProxyContainer(name)
	}
	for proxyname, _ := range c.proxies {
		if _, ok := c.desiredServices[proxyname]; !ok {
			log.Println("Undeploying proxy: ", proxyname)
			docker.StopContainer(proxyname, c.vanClient.DockerInterface)
			docker.RemoveContainer(proxyname, c.vanClient.DockerInterface)
		}
	}

	// TODO: when data plane is in qdr, we will likely need to manage aliases
	// for the service interface address e.g. reconcile actual services with desired
	// services:

	return nil
}

func (c *Controller) restoreControllerContext() error {

	svcDefs := make(map[string]types.ServiceInterface)

	svcFile, err := ioutil.ReadFile("/etc/messaging/services/all-skupper-services")
	if err != nil {
		return fmt.Errorf("Failed to retrieve skupper service interace definitions: %w", err)
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}
	for name, service := range svcDefs {
		c.desiredServices[name] = service
		c.byName[name] = service

		proxy, _ := docker.InspectContainer(name, c.vanClient.DockerInterface)
		c.proxies[name] = proxy

		if service.Origin != "" {
			if _, ok := c.byOrigin[service.Origin]; !ok {
				c.byOrigin[service.Origin] = make(map[string]types.ServiceInterface)
			}
			c.byOrigin[service.Origin][name] = service
		}

	}
	return nil
}

func (c *Controller) processUpdateWorkItem(syncUpdate chan *ServiceSyncUpdate) {
	var watcher *fsnotify.Watcher

	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	err := watcher.Add("/etc/messaging/services/local-skupper-services")
	if err != nil {
		log.Println("Could not add local services directory watcher", err.Error())
		return
	}

	for {
		select {
		case update, _ := <-syncUpdate:
			c.ensureServiceInterfaceDefinitions(update.origin, update.indexed)
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				svcDefs, err := getServices("local")
				if err != nil {
					log.Println("Failed to retreive service definitions: ", err.Error())
					return
				}
				indexed := make(map[string]types.ServiceInterface)
				for _, def := range svcDefs {
					def.Origin = c.origin
					indexed[def.Address] = def
				}
				c.ensureServiceInterfaceDefinitions(c.origin, indexed)
			} else {
				return
			}
		}
	}
}

func (c *Controller) processServiceWorkItem() {
	var watcher *fsnotify.Watcher

	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	err := watcher.Add("/etc/messaging/services/all-skupper-services")
	if err != nil {
		log.Println("Could not add local services directory watcher", err.Error())
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				svcDefs, err := getServices("all")
				if err != nil {
					log.Println("Failed to retrieve skupper service definitions: ", err.Error())
					return
				}
				definitions := make(map[string]types.ServiceInterface)
				if len(svcDefs) > 0 {
					for _, v := range svcDefs {
						definitions[v.Address] = v
					}
					c.desiredServices = definitions
					keys := []string{}
					for key, _ := range c.desiredServices {
						keys = append(keys, key)
					}
					log.Println("Desired service configuration updated: ", keys)
					c.reconcile()
				} else {
					c.desiredServices = definitions
					log.Println("No skupper services defined.")
					c.reconcile()
				}
				c.serviceSyncDefinitionsUpdated(definitions)
			} else {
				return
			}
		}
	}

}
