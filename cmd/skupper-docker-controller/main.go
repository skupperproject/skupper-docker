package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	amqp "github.com/Azure/go-amqp"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerfilters "github.com/docker/docker/api/types/filters"
	dockermounttypes "github.com/docker/docker/api/types/mount"
	dockernetworktypes "github.com/docker/docker/api/types/network"

	"github.com/fsnotify/fsnotify"
	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker/libdocker"
)

const (
	ServiceSyncAddress  = "mc/$skupper-service-sync"
	hostPath            = "/tmp/skupper"
	skupperCertPath     = hostPath + "/qpid-dispatch-certs/"
	skupperServicesPath = "/etc/messaging/services/"
)

var (
	svcAddressOrigin = make(map[string]string)
)

type ServiceSyncUpdate struct {
	origin  string
	indexed map[string]types.ServiceInterface
}

func NewServiceSyncUpdate() *ServiceSyncUpdate {
	var ssu ServiceSyncUpdate
	ssu.indexed = make(map[string]types.ServiceInterface)
	return &ssu
}

func describe(i interface{}) {
	fmt.Printf("(%v, %T)\n", i, i)
	fmt.Println()
}

func getTlsConfig(verify bool, cert, key, ca string) (*tls.Config, error) {
	var config tls.Config
	config.InsecureSkipVerify = true
	if verify {
		certPool := x509.NewCertPool()
		file, err := ioutil.ReadFile(ca)
		if err != nil {
			return nil, err
		}
		certPool.AppendCertsFromPEM(file)
		config.RootCAs = certPool
		config.InsecureSkipVerify = false
	}

	_, errCert := os.Stat(cert)
	_, errKey := os.Stat(key)
	if errCert == nil || errKey == nil {
		tlsCert, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			log.Fatal("Could not load x509 key pair", err.Error())
		}
		config.Certificates = []tls.Certificate{tlsCert}
	}
	config.MinVersion = tls.VersionTLS10

	return &config, nil
}

func authOption(username string, password string) amqp.ConnOption {
	if len(password) > 0 && len(username) > 0 {
		return amqp.ConnSASLPlain(username, password)
	} else {
		return amqp.ConnSASLAnonymous()
	}
}

func reconcileService(origin string, service types.ServiceInterface) {
	var update = true

	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	_, err := dd.InspectContainer(service.Address)
	if err == nil {
		return
	}

	if update {
		deploy(origin, service, dd)
	}
}

func ensureDefinitions(svcDefs *ServiceSyncUpdate) {
	// 1. If sync update service address present locally, ignore
	// 2. If sync update service address present remotely, but diff origin, ignore
	// 3. If origin current service(s) not in update, delete
	// 4. Otherwise, reconcile create versus update

	ensureOrigin := svcDefs.origin
	ensureUpdate := svcDefs.indexed

	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	localSvcs := getLocalServices()

	for address, service := range ensureUpdate {
		if _, ok := localSvcs[address]; ok {
			// clear out if necessary
			if _, ok := svcAddressOrigin[address]; ok {
				delete(svcAddressOrigin, address)
			}
			continue
		}
		if _, ok := svcAddressOrigin[address]; !ok {
			svcAddressOrigin[address] = ensureOrigin
			reconcileService(ensureOrigin, service)
			continue
		} else {
			if ensureOrigin != svcAddressOrigin[address] {
				continue
			} else {
				svcAddressOrigin[address] = ensureOrigin
				reconcileService(ensureOrigin, service)
				continue
			}
		}
	}

	// check against current origin services to see if we need to delete anything
	for address, origin := range svcAddressOrigin {
		if origin == ensureOrigin {
			if _, ok := ensureUpdate[address]; !ok {
				delete(svcAddressOrigin, address)
				undeploy(address, dd)
			}
		}
	}
}

func getOriginServices(origin string, dd libdocker.Interface) map[string]types.ServiceInterface {
	// get list of proxy containers for origin
	items := make(map[string]types.ServiceInterface)

	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/application")

	opts := dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	}

	containers, err := dd.ListContainers(opts)
	if err != nil {
		log.Fatal("Failed to list proxy containers: ", err.Error())
	}

	for _, container := range containers {
		if value, ok := container.Labels["skupper.io/origin"]; ok {
			svc := types.ServiceInterface{}
			if value == origin {
				last := container.Labels["skupper.io/last-applied"]
				err := json.Unmarshal([]byte(last), &svc)
				if err != nil {
					log.Println("Error decoding container label: ", err.Error())
				} else {
					items[svc.Address] = svc
				}
			}
		}
	}
	return items
}

func getRemoteServices(dd libdocker.Interface) map[string]types.ServiceInterface {
	// get list of non-locally originated proxy containers
	items := make(map[string]types.ServiceInterface)

	localOrigin := os.Getenv("SKUPPER_SERVICE_SYNC_ORIGIN")

	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/application")

	opts := dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	}

	containers, err := dd.ListContainers(opts)
	if err != nil {
		log.Fatal("Failed to list proxy containers: ", err.Error())
	}

	for _, container := range containers {
		if value, ok := container.Labels["skupper.io/origin"]; ok {
			svc := types.ServiceInterface{}
			if value != localOrigin {
				last := container.Labels["skupper.io/last-applied"]
				err := json.Unmarshal([]byte(last), &svc)
				if err != nil {
					log.Println("Error decoding container label: ", err.Error())
				} else {
					items[svc.Address] = svc
				}
			}
		}
	}
	return items
}

func getLocalServices() map[string]types.ServiceInterface {
	items := make(map[string]types.ServiceInterface)

	files, err := ioutil.ReadDir("/etc/messaging/services")
	if err == nil {
		for _, file := range files {
			svc := types.ServiceInterface{}
			data, err := ioutil.ReadFile("/etc/messaging/services/" + file.Name())
			if err == nil {
				err = json.Unmarshal(data, &svc)
				if err == nil {
					// do not convey target configuration
					svc.Targets = nil
					items[svc.Address] = svc
					//items = append(items, svc)
				} else {
					log.Println("Error unmarshalling local services file: ", err.Error())
				}
			} else {
				log.Println("Error reading local services file: ", err.Error())
			}
		}
	} else {
		log.Println("Error reading local services directory", err.Error())
	}
	return items
}

func getLabels(origin string, service types.ServiceInterface, isLocal bool) map[string]string {
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
		"skupper.io/origin":       origin,
		"skupper.io/last-applied": string(lastApplied),
	}
}

func deploy(origin string, service types.ServiceInterface, dd libdocker.Interface) {
	isLocal := true
	myOrigin := os.Getenv("SKUPPER_SERVICE_SYNC_ORIGIN")
	if origin != myOrigin {
		isLocal = false
	}

	var imageName string
	if os.Getenv("PROXY_IMAGE") != "" {
		imageName = os.Getenv("PROXY_IMAGE")
	} else {
		imageName = "quay.io/ajssmith/icproxy-simple"
	}
	err := dd.PullImage(imageName, dockertypes.AuthConfig{}, dockertypes.ImagePullOptions{})
	if err != nil {
		log.Fatal("Failed to pull proxy image: ", err.Error())
	}

	hostService := service.Targets[0].Selector == "internal.skupper.io/hostservice"

	// attach target container to the skupper network
	if isLocal && !hostService {
		//	if service.Targets[0].Name != "" {
		err := dd.ConnectContainerToNetwork("skupper-network", service.Targets[0].Name)
		if err != nil {
			log.Fatal("Failed to attach target container to skupper network: ", err.Error())
		}
	}

	labels := getLabels(origin, service, isLocal)
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
				Source: skupperCertPath + "skupper",
				Target: "/etc/messaging",
			},
		},
		Privileged: true,
	}
	networkCfg := &dockernetworktypes.NetworkingConfig{
		EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
			"skupper-network": {},
		},
	}

	opts := &dockertypes.ContainerCreateConfig{
		Name:             service.Address,
		Config:           containerCfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkCfg,
	}
	_, err = dd.CreateContainer(*opts)
	if err != nil {
		log.Fatal("Failed to create proxy container: ", err.Error())
	}
	err = dd.StartContainer(opts.Name)
	if err != nil {
		log.Fatal("Failed to start proxy container: ", err.Error())
	}
}

func undeploy(name string, dd libdocker.Interface) {
	err := dd.StopContainer(name, 10*time.Second)
	if err == nil {
		if err := dd.RemoveContainer(name, dockertypes.ContainerRemoveOptions{}); err != nil {
			log.Fatal("Failed to remove proxy container: ", err.Error())
		}
	}
}

func deployLocalService(name string) {
	var svc types.ServiceInterface

	origin := os.Getenv("SKUPPER_SERVICE_SYNC_ORIGIN")

	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)
	file, err := ioutil.ReadFile(name)
	if err != nil {
		log.Println("Local service file could not be read", err.Error())
	} else {
		err := json.Unmarshal(file, &svc)
		if err != nil {
			log.Println("Error decoding services file", err.Error())
			return
		}

		// Check for non-local address, if so override
		address := strings.TrimPrefix(name, "/etc/messaging/services/")
		_, err = dd.InspectContainer(address)
		if err == nil {
			undeploy(address, dd)
		}
		deploy(origin, svc, dd)
	}
}

func undeployLocalService(address string) {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	// file is removed so we have to inspect the container to get the label
	// of the target container that we need to disconnect from skupper network
	existing, err := dd.InspectContainer(address)
	if err != nil {
		log.Println("Local service container could not be retrieved", err.Error())
	} else {
		if target, ok := existing.Config.Labels["skupper.io/target"]; ok {
			if target != "" {
				if targetType, ok := existing.Config.Labels["skupper.io/targetType"]; ok {
					if targetType == "container" {
						err := dd.DisconnectContainerFromNetwork("skupper-network", target, true)
						if err != nil {
							log.Fatal("Failed to detatch target container from skupper network: ", err.Error())
						}
					}
				}
			}
		}
		undeploy(address, dd)
	}
}

func syncSender(s *amqp.Session, sendLocal chan bool) {
	var request amqp.Message
	var properties amqp.MessageProperties

	myOrigin := os.Getenv("SKUPPER_SERVICE_SYNC_ORIGIN")

	ctx := context.Background()
	sender, err := s.NewSender(
		amqp.LinkTargetAddress(ServiceSyncAddress),
	)
	if err != nil {
		log.Fatal("Failed to create sender:", err)
	}

	defer func() {
		sender.Close(ctx)
	}()

	ticker := time.NewTicker(10 * time.Second)

	properties.Subject = "service-sync-request"
	request.Properties = &properties
	request.ApplicationProperties = make(map[string]interface{})
	request.ApplicationProperties["origin"] = myOrigin

	for {
		select {
		case <-sendLocal:
			properties.Subject = "service-sync-update"
			svcDefsMap := getLocalServices()
			svcDefs := []types.ServiceInterface{}
			if len(svcDefsMap) > 0 {
				for _, svc := range svcDefsMap {
					svc.Targets = nil
					svcDefs = append(svcDefs, svc)
				}
				encoded, err := json.Marshal(svcDefs)
				if err != nil {
					log.Println("Failed to create json for service definition sync: ", err.Error())
					return
				} else {
					fmt.Println("Sending local service definitions: ", string(encoded))
				}
				request.Value = string(encoded)
				err = sender.Send(ctx, &request)
			}
		case <-ticker.C:
			properties.Subject = "service-sync-request"
			request.Value = ""
			err = sender.Send(ctx, &request)
			fmt.Println("Skupper service request sent")
		}
	}

}

func watchForLocal(path string, syncUpdate chan *ServiceSyncUpdate) {
	var watcher *fsnotify.Watcher

	//myOrigin := os.Getenv("SKUPPER_SERVICE_SYNC_ORIGIN")

	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	err := watcher.Add(path)
	if err != nil {
		log.Fatal("Could not add local services directory watcher", err.Error())
	}

	for {
		select {
		case svcDefs, _ := <-syncUpdate:
			fmt.Println("Service sync update: ", svcDefs)
			ensureDefinitions(svcDefs)
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				fmt.Println("Sync local new file: ", event.Name)
			} else if event.Op&fsnotify.Write == fsnotify.Write {
				fmt.Println("Sync local modified file: ", event.Name)
				deployLocalService(event.Name)
			} else if event.Op&fsnotify.Remove == fsnotify.Remove {
				fmt.Println("Sync local removed file: ", event.Name)
				address := strings.TrimPrefix(event.Name, "/etc/messaging/services/")
				undeployLocalService(address)
			} else {
				return
			}
		}
	}
}

func main() {
	localOrigin := os.Getenv("SKUPPER_SERVICE_SYNC_ORIGIN")

	log.Println("Skupper service sync starting, local origin => ", localOrigin)

	config, err := getTlsConfig(true, "/etc/messaging/tls.crt", "/etc/messaging/tls.key", "/etc/messaging/ca.crt")

	client, err := amqp.Dial("amqps://skupper-router:5671", amqp.ConnSASLAnonymous(), amqp.ConnMaxFrameSize(4294967295), amqp.ConnTLSConfig(config))
	if err != nil {
		log.Fatal("Failed to connect to url", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Fatal("Failed to create session:", err)
	}

	sendLocal := make(chan bool)
	syncUpdate := make(chan *ServiceSyncUpdate)

	go syncSender(session, sendLocal)
	go watchForLocal(skupperServicesPath, syncUpdate)

	ctx := context.Background()
	receiver, err := session.NewReceiver(
		amqp.LinkSourceAddress(ServiceSyncAddress),
		amqp.LinkCredit(10),
	)
	if err != nil {
		log.Fatal("Failed to create receiver:", err.Error())
	}
	defer func() {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		receiver.Close(ctx)
		cancel()
	}()

	for {
		var ok bool
		msg, err := receiver.Receive(ctx)
		if err != nil {
			log.Fatal("Reading message from service synch: ", err.Error())
		}
		// Decode message as it is either a request to send update
		// or it is a receipt that needs to be reconciled
		msg.Accept()
		subject := msg.Properties.Subject

		if subject == "service-sync-request" {
			sendLocal <- true
		} else if subject == "service-sync-update" {
			svcSyncUpdate := NewServiceSyncUpdate()
			if svcSyncUpdate.origin, ok = msg.ApplicationProperties["origin"].(string); ok {
				if svcSyncUpdate.origin != localOrigin {
					if updates, ok := msg.Value.(string); ok {
						defs := []types.ServiceInterface{}
						err := json.Unmarshal([]byte(updates), &defs)
						if err == nil {
							for _, def := range defs {
								svcSyncUpdate.indexed[def.Address] = def
							}
							syncUpdate <- svcSyncUpdate
						} else {
							log.Println("Error marshall sawyer", err.Error())
						}
					}
				}
			} else {
				log.Println("Skupper service sync update type assertion error")
			}
		} else {
			log.Println("Service sync subject not valid")
		}
	}
	log.Println("Skupper service sync terminating")

}
