package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerfilters "github.com/docker/docker/api/types/filters"
	dockermounttypes "github.com/docker/docker/api/types/mount"
	dockernetworktypes "github.com/docker/docker/api/types/network"
	dockerapi "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"golang.org/x/net/context"

	"github.com/skupperproject/skupper-cli/pkg/certs"
	"github.com/skupperproject/skupper-docker/pkg/dockershim/libdocker"
	"github.com/skupperproject/skupper-docker/pkg/router"
	skupperservice "github.com/skupperproject/skupper-docker/pkg/service"
	"github.com/spf13/cobra"
)

const (
	version                 = "undefined"
	hostPath                = "/tmp/skupper"
	skupperCertPath         = hostPath + "/qpid-dispatch-certs/"
	skupperConnPath         = hostPath + "/connections/"
	skupperConsoleUsersPath = hostPath + "/console-users/"
	skupperSaslConfigPath   = hostPath + "/sasl-config/"
	skupperServicePath      = hostPath + "/services/"
)

type RouterMode string

const (
	RouterModeInterior RouterMode = "interior"
	RouterModeEdge                = "edge"
)

type ConnectorRole string

const (
	ConnectorRoleInterRouter ConnectorRole = "inter-router"
	ConnectorRoleEdge                      = "edge"
)

func connectJson() string {
	connect_json := `
{
    "scheme": "amqps",
    "host": "skupper-router",
    "port": "5671",
    "tls": {
        "ca": "/etc/messaging/ca.crt",
        "cert": "/etc/messaging/tls.crt",
        "key": "/etc/messaging/tls.key",
        "verify": true
    }
}
`
	return connect_json
}

type ConsoleAuthMode string

const (
	ConsoleAuthModeInternal  ConsoleAuthMode = "internal"
	ConsoleAuthModeUnsecured                 = "unsecured"
)

type Router struct {
	Name            string
	Mode            RouterMode
	Console         ConsoleAuthMode
	ConsoleUser     string
	ConsolePassword string
}

type DockerDetails struct {
	Cli *dockerapi.Client
	Ctx context.Context
}

func routerConfig(router *Router) string {
	config := `
router {
    mode: {{.Mode}}
    id: {{.Name}}-${HOSTNAME}
}

listener {
    host: localhost
    port: 5672
    role: normal
}

sslProfile {
    name: skupper-amqps
    certFile: /etc/qpid-dispatch-certs/skupper-amqps/tls.crt
    privateKeyFile: /etc/qpid-dispatch-certs/skupper-amqps/tls.key
    caCertFile: /etc/qpid-dispatch-certs/skupper-amqps/ca.crt
}

listener {
    host: 0.0.0.0
    port: 5671
    role: normal
    sslProfile: skupper-amqps
    saslMechanisms: ANONYMOUS PLAIN
    authenticatePeer: false
}

{{- if eq .Console "openshift"}}
# console secured by oauth proxy sidecar
listener {
    host: localhost
    port: 8888
    role: normal
    http: true
}
{{- else if eq .Console "internal"}}
listener {
    host: 0.0.0.0
    port: 8080
    role: normal
    http: true
    authenticatePeer: true
}
{{- else if eq .Console "unsecured"}}
listener {
    host: 0.0.0.0
    port: 8080
    role: normal
    http: true
}
{{- end }}

listener {
    host: 0.0.0.0
    port: 9090
    role: normal
    http: true
    httpRootDir: disabled
    websockets: false
    healthz: true
    metrics: true
}

{{- if eq .Mode "interior" }}
sslProfile {
    name: skupper-internal
    certFile: /etc/qpid-dispatch-certs/skupper-internal/tls.crt
    privateKeyFile: /etc/qpid-dispatch-certs/skupper-internal/tls.key
    caCertFile: /etc/qpid-dispatch-certs/skupper-internal/ca.crt
}

listener {
    role: inter-router
    host: 0.0.0.0
    port: 55671
    sslProfile: skupper-internal
    saslMechanisms: EXTERNAL
    authenticatePeer: true
}

listener {
    role: edge
    host: 0.0.0.0
    port: 45671
    sslProfile: skupper-internal
    saslMechanisms: EXTERNAL
    authenticatePeer: true
}
{{- end}}

address {
    prefix: mc
    distribution: multicast
}

## Connectors: ##
`
	var buff bytes.Buffer
	qdrconfig := template.Must(template.New("qdrconfig").Parse(config))
	qdrconfig.Execute(&buff, router)
	return buff.String()
}

type Connector struct {
	Name string
	Host string
	Port string
	Role ConnectorRole
	Cost int
}

func connectorConfig(connector *Connector) string {
	config := `

sslProfile {
    name: {{.Name}}-profile
    certFile: /etc/qpid-dispatch/connections/{{.Name}}/tls.crt
    privateKeyFile: /etc/qpid-dispatch/connections/{{.Name}}/tls.key
    caCertFile: /etc/qpid-dispatch/connections/{{.Name}}/ca.crt
}

connector {
    name: {{.Name}}-connector
    host: {{.Host}}
    port: {{.Port}}
    role: {{.Role}}
    cost: {{.Cost}}
    sslProfile: {{.Name}}-profile
}

`
	var buff bytes.Buffer
	connectorconfig := template.Must(template.New("connectorconfig").Parse(config))
	connectorconfig.Execute(&buff, connector)
	return buff.String()
}

func ensureSaslUsers(user string, password string) {
	_, err := ioutil.ReadFile(skupperConsoleUsersPath + user)
	if err == nil {
		log.Println("Console user already exists: ", user)
	} else {
		if err := ioutil.WriteFile(skupperConsoleUsersPath+user, []byte(password), 0755); err != nil {
			log.Fatal("Failed to write console user password file: ", err.Error())
		}
	}
}

func ensureSaslConfig() {
	name := "qdrouterd.conf"

	_, err := ioutil.ReadFile(skupperSaslConfigPath + name)
	if err == nil {
		log.Println("sasl config file already exists: ", skupperSaslConfigPath + name)
	} else {
		config := `
pwcheck_method: auxprop
auxprop_plugin: sasldb
sasldb_path: /tmp/qdrouterd.sasldb
`
		if err := ioutil.WriteFile(skupperSaslConfigPath+name, []byte(config), 0755); err != nil {
			log.Fatal("Failed to write sasl config file: ", err.Error())
		}
	}
}

func routerPorts(router *Router) nat.PortSet {
	ports := nat.PortSet{}

	ports["9090/tcp"] = struct{}{}
	if router.Console != "" {
		ports["8080/tcp"] = struct{}{}
	}
	if router.Mode == RouterModeInterior {
		// inter-router
		ports["55671/tcp"] = struct{}{}
		// edge
		ports["45671/tcp"] = struct{}{}
	}
	return ports
}

func findEnvVar(env []string, name string) string {
	for _, v := range env {
		if strings.HasPrefix(v, name) {
			return strings.TrimPrefix(v, name+"=")
		}
	}
	return ""
}

func setEnvVar(current []string, name string, value string) []string {
	updated := []string{}
	for _, v := range current {
		if strings.HasPrefix(v, name) {
			updated = append(updated, name+"="+value)
		} else {
			updated = append(updated, v)
		}
	}
	return updated
}

func isInterior(tcj *dockertypes.ContainerJSON) bool {
	config := findEnvVar(tcj.Config.Env, "QDROUTERD_CONF")
	if config == "" {
		log.Fatal("Could not retrieve router config")
	}
	match, _ := regexp.MatchString("mode:[ ]+interior", config)
	return match
}

func generateConnectorName(path string) string {
	files, err := ioutil.ReadDir(path)
	max := 1
	if err == nil {
		connectorNamePattern := regexp.MustCompile("conn([0-9])+")
		for _, f := range files {
			count := connectorNamePattern.FindStringSubmatch(f.Name())
			if len(count) > 1 {
				v, _ := strconv.Atoi(count[1])
				if v >= max {
					max = v + 1
				}
			}
		}
	} else {
		log.Fatal("Could not retrieve configured connectors (need init?): ", err.Error())
	}
	return "conn" + strconv.Itoa(max)
}

func getRouterMode(tcj *dockertypes.ContainerJSON) RouterMode {
	if isInterior(tcj) {
		return RouterModeInterior
	} else {
		return RouterModeEdge
	}
}

func routerEnv(router *Router) []string {
	envVars := []string{}

	if router.Mode == RouterModeInterior {
		envVars = append(envVars, "APPLICATION_NAME=skupper-router")
	}
	envVars = append(envVars, "QDROUTERD_CONF="+routerConfig(router))
	envVars = append(envVars, "PN_TRACE_FRM=1")
	if router.Console == ConsoleAuthModeInternal {
		envVars = append(envVars, "QDROUTERD_AUTO_CREATE_SASLDB_SOURCE=/etc/qpid-dispatch/sasl-users/")
		envVars = append(envVars, "QDROUTERD_AUTO_CREATE_SASLDB_PATH=/tmp/qdrouterd.sasldb")
	}

	return envVars
}

func syncEnv(name string) []string {
	envVars := []string{}
	if name != "" {
		envVars = append(envVars, "ICPROXY_BRIDGE_HOST="+name)
	}
	return envVars
}

func proxyEnv(service skupperservice.Service) []string {
	envVars := []string{}
	if service.Targets[0].Name != "" {
		envVars = append(envVars, "ICPROXY_BRIDGE_HOST="+service.Targets[0].Name)
	}
	return envVars
}

func getLabels(component string, service skupperservice.Service) map[string]string {
	application := "skupper"
	if component == "router" {
		//the automeshing function of the router image expects the application
		//to be used as a unique label for identifying routers to connect to
		return map[string]string{
			"application":          "skupper-router",
			"skupper.io/component": component,
			"prometheus.io/port":   "9090",
			"prometheus.io/scrape": "true",
		}
	} else if component == "proxy" {
		return map[string]string{
			"application":          "skupper-proxy",
			"skupper.io/component": component,
			"skupper.io/process":   service.Targets[0].Name,
		}
	} else if component == "network" {
		return map[string]string{
			"application":          "skupper-network",
			"skupper.io/component": component,
		}
	}
	return map[string]string{
		"application": application,
	}
}

func randomId(length int) string {
	buffer := make([]byte, length)
	rand.Read(buffer)
	result := base64.StdEncoding.EncodeToString(buffer)
	return result[:length]
}

func getCertData(name string) (certs.CertificateData, error) {
	certData := certs.CertificateData{}
	certPath := skupperCertPath + name

	files, err := ioutil.ReadDir(certPath)
	if err == nil {
		for _, f := range files {
			dataString, err := ioutil.ReadFile(certPath + "/" + f.Name())
			if err == nil {
				certData[f.Name()] = []byte(dataString)
			} else {
				log.Fatal("Failed to read certificat data: ", err.Error())
			}
		}
	}

	return certData, err
}

func ensureCA(name string) certs.CertificateData {
	// check if existing by looking at path/dir, if not create dir to persist
	caData := certs.GenerateCACertificateData(name, name)

	if err := os.Mkdir(skupperCertPath+name, 0755); err != nil {
		log.Fatal("Failed to create certificate directory: ", err.Error())
	}

	for k, v := range caData {
		if err := ioutil.WriteFile(skupperCertPath+name+"/"+k, v, 0755); err != nil {
			log.Fatal("Failed to write CA certificate file: ", err.Error())
		}
	}

	return caData
}

func generateCredentials(caData certs.CertificateData, name string, subject string, hosts string, includeConnectJson bool) {
	certData := certs.GenerateCertificateData(name, subject, hosts, caData)

	for k, v := range certData {
		if err := ioutil.WriteFile(skupperCertPath+name+"/"+k, v, 0755); err != nil {
			log.Fatal("Failed to write certificate file: ", err.Error())
		}
	}

	if includeConnectJson {
		certData["connect.json"] = []byte(connectJson())
		if err := ioutil.WriteFile(skupperCertPath+name+"/connect.json", []byte(connectJson()), 0755); err != nil {
			log.Fatal("Failed to write connect file: ", err.Error())
		}
	}

}

func createRouterHostFiles(volumes []string) {

	// only called during init, remove previous files and start anew
	_ = os.RemoveAll(hostPath)
	if err := os.MkdirAll(hostPath, 0755); err != nil {
		log.Fatal("Failed to create skupper host directory: ", err.Error())
	}
	if err := os.Mkdir(skupperCertPath, 0755); err != nil {
		log.Fatal("Failed to create skupper host directory: ", err.Error())
	}
	if err := os.Mkdir(skupperConnPath, 0755); err != nil {
		log.Fatal("Failed to create skupper host directory: ", err.Error())
	}
	if err := os.Mkdir(skupperConsoleUsersPath, 0777); err != nil {
		log.Fatal("Failed to create skupper host directory: ", err.Error())
	}
	if err := os.Mkdir(skupperServicePath, 0755); err != nil {
		log.Fatal("Failed to create skupper host directory: ", err.Error())
	}
	if err := os.Mkdir(skupperSaslConfigPath, 0755); err != nil {
		log.Fatal("Failed to create skupper host directory: ", err.Error())
	}
	for _, v := range volumes {
		if err := os.Mkdir(skupperCertPath+v, 0755); err != nil {
			log.Fatal("Failed to create skupper host directory: ", err.Error())
		}
	}

}

func routerHostConfig(router *Router) *dockercontainer.HostConfig {

	hostcfg := &dockercontainer.HostConfig{
		Mounts: []dockermounttypes.Mount{
			{
				Type:   dockermounttypes.TypeBind,
				Source: skupperConnPath,
				Target: "/etc/qpid-dispatch/connections",
			},
			{
				Type:   dockermounttypes.TypeBind,
				Source: skupperCertPath,
				Target: "/etc/qpid-dispatch-certs",
			},
			{
				Type:   dockermounttypes.TypeBind,
				Source: skupperConsoleUsersPath,
				Target: "/etc/qpid-dispatch/sasl-users/",
			},
			{
				Type:   dockermounttypes.TypeBind,
				Source: skupperSaslConfigPath,
				Target: "/etc/sasl2",
			},
		},
		Privileged: true,
	}
	return hostcfg
}

func routerContainerConfig(router *Router) *dockercontainer.Config {
	var image string

	labels := getLabels("router", skupperservice.Service{})

	if os.Getenv("QDROUTERD_IMAGE") != "" {
		image = os.Getenv("QDROUTERD_IMAGE")
	} else {
		image = "quay.io/interconnectedcloud/qdrouterd"
	}
	cfg := &dockercontainer.Config{
		Hostname: "skupper-router",
		Image:    image,
		Env:      routerEnv(router),
		Healthcheck: &dockercontainer.HealthConfig{
			Test:        []string{"curl --fail -s http://localhost:9090/healthz || exit 1"},
			StartPeriod: (time.Duration(60)*time.Second),
		},
		Labels:       labels,
		ExposedPorts: routerPorts(router),
	}

	return cfg
}

func routerNetworkConfig() *dockernetworktypes.NetworkingConfig {
	netcfg := &dockernetworktypes.NetworkingConfig{
		EndpointsConfig: map[string]*dockernetworktypes.EndpointSettings{
			"skupper-network": {},
		},
	}

	return netcfg
}

func makeRouterContainerCreateConfig(router *Router) *dockertypes.ContainerCreateConfig {

	opts := &dockertypes.ContainerCreateConfig{
		Name:             "skupper-router",
		Config:           routerContainerConfig(router),
		HostConfig:       routerHostConfig(router),
		NetworkingConfig: routerNetworkConfig(),
	}

	return opts
}

func restartRouterContainer(dd libdocker.Interface) {

	current, err := dd.InspectContainer("skupper-router")
	if err != nil {
		log.Fatal("Failed to retrieve router container (need init?): ", err.Error())
	} else {
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
		qdrConf := findEnvVar(currentEnv, "QDROUTERD_CONF")
		updated := strings.Split(qdrConf, pattern)[0] + pattern

		files, err := ioutil.ReadDir(skupperConnPath)
		for _, f := range files {
			connName := f.Name()
			hostString, _ := ioutil.ReadFile(skupperConnPath + connName + "/inter-router-host")
			portString, _ := ioutil.ReadFile(skupperConnPath + connName + "/inter-router-port")
			connector := Connector{
				Name: connName,
				Host: string(hostString),
				Port: string(portString),
				Role: ConnectorRoleInterRouter,
			}
			updated += connectorConfig(&connector)
		}

		newEnv := setEnvVar(currentEnv, "QDROUTERD_CONF", updated)

		containerCfg := &dockercontainer.Config{
			Hostname: current.Config.Hostname,
			Image:    current.Config.Image,
			Healthcheck: &dockercontainer.HealthConfig{
				Test:        []string{"curl --fail -s http://localhost:9090/healthz || exit 1"},
				StartPeriod: (time.Duration(60)*time.Second),
			},
			Labels:       current.Config.Labels,
			ExposedPorts: current.Config.ExposedPorts,
			Env:          newEnv,
		}

		// remove current and create new container
		err = dd.StopContainer("skupper-router", 10*time.Second)
		if err == nil {
			if err := dd.RemoveContainer("skupper-router", dockertypes.ContainerRemoveOptions{}); err != nil {
				log.Fatal("Failed to remove router container: ", err.Error())
			}
		}

		opts := &dockertypes.ContainerCreateConfig{
			Name:             "skupper-router",
			Config:           containerCfg,
			HostConfig:       hostCfg,
			NetworkingConfig: routerNetworkConfig(),
		}
		_, err = dd.CreateContainer(*opts)
		if err != nil {
			log.Fatal("Failed to re-create router container: ", err.Error())
		}

		err = dd.StartContainer(opts.Name)
		if err != nil {
			log.Fatal("Failed to re-start router container: ", err.Error())
		}

		err = dd.RestartContainer("skupper-proxy-controller", 10*time.Second)
		if err != nil {
			log.Fatal("Failed to re-start skupper-proxy-controller container: ", err.Error())
		}
		restartProxies(dd)
	}

}

func checkConnection(name string) bool {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	// check that init has fun first by inspecting skupper router container
	current, err := dd.InspectContainer("skupper-router")
	if err != nil {
		if dockerapi.IsErrNotFound(err) {
			log.Fatal("Router container does not exist (need init?): " + err.Error())
		} else {
			log.Fatal("Error retrieving router container: " + err.Error())
		}
		return false
	}
	mode := getRouterMode(current)
	var connectors []Connector
	if name == "all" {
		connectors = retrieveConnectors(mode, dd)
	} else {
		connector, err := getConnector(name, mode, dd)
		if err == nil {
			connectors = append(connectors, connector)
		} else {
			log.Printf("Could not find connector %s: %s", name, err.Error())			
			return false
		}
	}
	connections, err := router.GetConnections(dd)
    if err == nil {
		result := true
		for _, connector := range connectors {
			connection := router.GetInterRouterConnection(connector.Host + ":" + connector.Port, connections)
			if connection == nil || !connection.Active {
				fmt.Printf("Connection for %s not active", connector.Name)
				fmt.Println()
				result = false
			} else {
				fmt.Printf("Connection for %s is active", connector.Name)
				fmt.Println()
			}
		}
		return result        
    } else {
        fmt.Println("Could not check connections: ", err.Error())
        return false
    }

}

func getConnector(name string, mode RouterMode, dd libdocker.Interface) (Connector, error) {
	var role ConnectorRole
	var suffix string

	if mode == RouterModeEdge {
		role = ConnectorRoleEdge
		suffix = "/edge-"
	} else {
		role = ConnectorRoleInterRouter
		suffix = "/inter-router-"
	}
	host, err := ioutil.ReadFile(skupperConnPath + name + suffix + "host")
	if err != nil {
		log.Fatal("Could not retrieve connection-token files: ", err.Error())
		return Connector{}, err
	}
	port, err := ioutil.ReadFile(skupperConnPath + name + suffix + "port")
	if err != nil {
		log.Fatal("Could not retrieve connection-token files: ", err.Error())
		return Connector{}, err
	}
	connector := Connector{
		Name: name,
		Host: string(host),
		Port: string(port),
		Role: role,
	}

	return connector, nil
}

func retrieveConnectors(mode RouterMode, dd libdocker.Interface) []Connector {
	var connectors []Connector
	files, err := ioutil.ReadDir(skupperConnPath)
	if err == nil {
		var role ConnectorRole
		var host []byte
		var port []byte
		var suffix string
		if mode == RouterModeEdge {
			role = ConnectorRoleEdge
			suffix = "/edge-"
		} else {
			role = ConnectorRoleInterRouter
			suffix = "/inter-router-"
		}
		for _, f := range files {
			host, _ = ioutil.ReadFile(skupperConnPath + f.Name() + suffix + "host")
			port, _ = ioutil.ReadFile(skupperConnPath + f.Name() + suffix + "port")
			connectors = append(connectors, Connector{
				Name: f.Name(),
				Host: string(host),
				Port: string(port),
				Role: role,
			})
		}
	} else {
		log.Fatal("Could not retrieve connection-token files: ", err.Error())
	}
	return connectors
}

func generateConnectSecret(subject string, secretFile string) {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	// verify that the local deployment is interior
	current, err := dd.InspectContainer("skupper-router")
	if err == nil {
		if isInterior(current) {
			caData, err := getCertData("skupper-internal-ca")
			if err == nil {
				annotations := make(map[string]string)
				annotations["inter-router-port"] = "443"
				annotations["inter-router-host"] = string(current.NetworkSettings.IPAddress)

				certData := certs.GenerateCertificateData(subject, subject, string(current.NetworkSettings.IPAddress), caData)
				certs.PutCertificateData(subject, secretFile, certData, annotations)
			}
		} else {
			log.Fatal("Edge mode configuration cannot accept connections")
		}
	} else if dockerapi.IsErrNotFound(err) {
		log.Fatal("Router container does not exist (need init?): " + err.Error())
	} else {
		log.Fatal("Error retrieving router container: " + err.Error())
	}
}

func status(listConnectors bool) {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	current, err := dd.InspectContainer("skupper-router")
	if err == nil {
		mode := getRouterMode(current)
		var modedesc string
		if mode == RouterModeEdge {
			modedesc = "in edge mode"
		} else {
            modedesc = "in interior mode"
        }
        connected, err := router.GetConnectedSites(dd)
		for i :=0; i < 5 && err != nil; i++ {        
            time.Sleep(500*time.Millisecond)
            connected, err = router.GetConnectedSites(dd)
        }
        if err != nil {
            fmt.Printf("Skupper enabled %s. Unable to determine connectivity:%s", modedesc, err.Error())
        } else {
            fmt.Printf("Skupper enabled %s.", modedesc)
            if connected.Total == 0 {
                fmt.Printf(" It is not connected to any other sites.")
            } else if connected.Total == 1 {
                fmt.Printf(" It is connected to 1 other site.")
            } else if connected.Total == connected.Direct {
                fmt.Printf(" It is connected to %d other sites.", connected.Total)
            } else {
                fmt.Printf(" It is connected to %d other sites (%d indirectly).", connected.Total, connected.Indirect)
            }
        }
        exposed := countServiceDefinitions()
    	if exposed == 1 {
			fmt.Printf(" 1 service is exposed.")
		} else if exposed > 0 {
			fmt.Printf(" %d services are exposed.", exposed)
		}
        fmt.Println()
	} else if dockerapi.IsErrNotFound(err) {
		fmt.Println("Skupper is not currently enabled")
	} else {
		log.Fatal(err.Error())
	}

}

func delete() {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	_, err := dd.InspectContainer("skupper-proxy-controller")
	if err == nil {
		dd.StopContainer("skupper-proxy-controller", 10*time.Second)
		if err != nil {
			log.Fatal("Failed to stop proxy controller container: ", err.Error())
		}
		err = dd.RemoveContainer("skupper-proxy-controller", dockertypes.ContainerRemoveOptions{})
		if err != nil {
			log.Fatal("Failed to remove proxy controller container: ", err.Error())
		}
	}

	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/component")

	opts := dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	}
	containers, err := dd.ListContainers(opts)
	if err != nil {
		log.Fatal("Failed to list proxy containers: ", err.Error())
	}

	fmt.Println("Stopping skupper proxy containers...")
	for _, container := range containers {
		if value, ok := container.Labels["skupper.io/component"]; ok {
			if value == "proxy" {
				err := dd.StopContainer(container.ID, 10*time.Second)
				if err != nil {
					log.Fatal("Failed to stop proxy container: ", err.Error())
				}
				err = dd.RemoveContainer(container.ID, dockertypes.ContainerRemoveOptions{})
				if err != nil {
					log.Fatal("Failed to remove proxy container: ", err.Error())
				}
			}
		}
	}

	fmt.Println("Stopping skupper-router container...")
	err = dd.StopContainer("skupper-router", 10*time.Second)
	if err == nil {
		err := dd.RemoveContainer("skupper-router", dockertypes.ContainerRemoveOptions{})
		if err != nil {
			log.Fatal("Failed to remove router container: ", err.Error())
		}
	} else if dockerapi.IsErrNotFound(err) {
		log.Fatal("Router container does not exist")
	} else {
		log.Fatal("Failed to stop router container: ", err.Error())
	}

	fmt.Println("Removing skupper-network network...")
	// first remove any containers that remain on the network
	tnr, err := dd.InspectNetwork("skupper-network")
	if err == nil {
		for _, container := range tnr.Containers {
			err := dd.DisconnectContainerFromNetwork("skupper-network", container.Name, true)
			if err != nil {
				log.Fatal("Failed to disconnect container from skupper-network: ", err.Error())
			}
		}
	}
	err = dd.RemoveNetwork("skupper-network")
	if err != nil {
		log.Fatal("Failed to remove skupper-network network: ", err.Error())
	}

	// Removing files and directory...
	err = os.RemoveAll(hostPath)
	if err != nil {
		log.Fatal("Failed to remove skupper files and directory: ", err.Error())
	}
	fmt.Println("Skupper resources now removed")
}

func connect(secretFile string, connectorName string, cost int) {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)
	// TODO: how to detect duplicate connection tokens?
	// examine host and port for each configured connector, should not collide

	secret := certs.GetSecretContent(secretFile)
	if secret != nil {
		existing, err := dd.InspectContainer("skupper-router")
		if err == nil {
			mode := getRouterMode(existing)

			if connectorName == "" {
				connectorName = generateConnectorName(skupperConnPath)
			}
			connPath := skupperConnPath + connectorName
			if err := os.Mkdir(connPath, 0755); err != nil {
				log.Fatal("Failed to create skupper connector directory: ", err.Error())
			}
			for k, v := range secret {
				if err := ioutil.WriteFile(connPath+"/"+k, v, 0755); err != nil {
					log.Fatal("Failed to write connector certificate file: ", err.Error())
				}
			}
			//read annotation files to get the host and port to connect to
			// TODO: error handling
			connector := Connector{
				Name: connectorName,
				Cost: cost,
			}
			if mode == RouterModeInterior {
				hostString, _ := ioutil.ReadFile(connPath + "/inter-router-host")
				portString, _ := ioutil.ReadFile(connPath + "/inter-router-port")
				connector.Host = string(hostString)
				connector.Port = string(portString)
				connector.Role = ConnectorRoleInterRouter
			} else {
				hostString, _ := ioutil.ReadFile(connPath + "/edge-host")
				portString, _ := ioutil.ReadFile(connPath + "/edge-port")
				connector.Host = string(hostString)
				connector.Port = string(portString)
				connector.Role = ConnectorRoleEdge
			}
			fmt.Printf("Skupper configured to connect to %s:%s (name=%s)", connector.Host, connector.Port, connector.Name)
			fmt.Println()
			restartRouterContainer(dd)
		} else {
			log.Fatal("Failed to retrieve router container (need init?): ", err.Error())
		}
	} else {
		log.Fatal("Failed to make connector, missing connection-token content")
	}
}

func disconnect(name string) {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)
	_, err := dd.InspectContainer("skupper-router")
	if err == nil {
		err = os.RemoveAll(skupperConnPath + name)
		restartRouterContainer(dd)
	} else {
		log.Fatal("Failed to retrieve router container (need init?): ", err.Error())
	}
}

func skupperInit(router *Router) {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	if router.Name == "" {
		info, err := dd.Info()
		if err != nil {
			log.Fatal("Failed to retrieve docker client info: ", err.Error())
		} else {
			router.Name = info.Name
		}
	}

	imageName := "quay.io/interconnectedcloud/qdrouterd"
	err := dd.PullImage(imageName, dockertypes.AuthConfig{}, dockertypes.ImagePullOptions{})
	if err != nil {
		log.Fatal("Failed to pull qdrouterd image: ", err.Error())
	}

	_, err = dd.CreateNetwork("skupper-network")
	if err != nil {
		log.Fatal("Failed to create skupper network: ", err.Error())
	}

	volumes := []string{"skupper", "skupper-amqps"}
	if router.Mode == RouterModeInterior {
		volumes = append(volumes, "skupper-internal")
	}

	createRouterHostFiles(volumes)

	opts := makeRouterContainerCreateConfig(router)
	_, err = dd.CreateContainer(*opts)
	if err != nil {
		log.Fatal("Failed to create skupper router container: ", err.Error())
	}

	err = dd.StartContainer(opts.Name)
	if err != nil {
		log.Fatal("Failed to start skupper router container: ", err.Error())
	}

	caData := ensureCA("skupper-ca")
	generateCredentials(caData, "skupper-amqps", opts.Name, opts.Name, false)
	generateCredentials(caData, "skupper", opts.Name, "", true)
	if router.Mode == RouterModeInterior {
		internalCaData := ensureCA("skupper-internal-ca")
		generateCredentials(internalCaData, "skupper-internal", "skupper-internal", "skupper-internal", false)
	}

	if router.Console == ConsoleAuthModeInternal {
		ensureSaslConfig()
		ensureSaslUsers(router.ConsoleUser, router.ConsolePassword)
	}

	startServiceSync(dd)
}

func restartProxies(dd libdocker.Interface) {

	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/component")

	containers, err := dd.ListContainers(dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	})
	if err != nil {
		log.Fatal("Failed to list proxy containers: ", err.Error())
	}

	for _, container := range containers {
		labels := container.Labels
		for k, v := range labels {
			if k == "skupper.io/component" && v == "proxy" {

				err = dd.RestartContainer(container.ID, 10*time.Second)
				if err != nil {
					log.Fatal("Failed to restart proxy container: ", err.Error())
				}
			}
		}
	}

}

func startServiceSync(dd libdocker.Interface) {
	_, err := dd.InspectContainer("skupper-proxy-controller")

	origin := randomId(10)

	if err == nil {
		fmt.Println("Container skupper-proxy-controller already exists")
		return
	} else if dockerapi.IsErrNotFound(err) {
		var imageName string
		if os.Getenv("SERVICE_SYNC_IMAGE") != "" {
			imageName = os.Getenv("PROXY_CONTROLLER_IMAGE")
		} else {
			imageName = "quay.io/skupper/skupper-docker-controller"
		}
		err := dd.PullImage(imageName, dockertypes.AuthConfig{}, dockertypes.ImagePullOptions{})
		if err != nil {
			log.Fatal("Failed to pull service sync image: ", err.Error())
		}

		containerCfg := &dockercontainer.Config{
			Hostname: "skupper-proxy-controller",
			Image:    imageName,
			Cmd:      []string{"/go/src/app/controller"},
			Env:      []string{"SKUPPER_SERVICE_SYNC_ORIGIN=" + origin},
		}
		hostCfg := &dockercontainer.HostConfig{
			Mounts: []dockermounttypes.Mount{
				{
					Type:   dockermounttypes.TypeBind,
					Source: skupperCertPath + "skupper",
					Target: "/etc/messaging",
				},
				{
					Type:   dockermounttypes.TypeBind,
					Source: skupperServicePath,
					Target: "/etc/messaging/services",
				},
				{
					Type:   dockermounttypes.TypeBind,
					Source: "/var/run",
					Target: "/var/run",
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
			Name:             "skupper-proxy-controller",
			Config:           containerCfg,
			HostConfig:       hostCfg,
			NetworkingConfig: networkCfg,
		}
		_, err = dd.CreateContainer(*opts)
		if err != nil {
			log.Fatal("Failed to create proxy controller container: ", err.Error())
		}
		err = dd.StartContainer(opts.Name)
		if err != nil {
			log.Fatal("Failed to start proxy controller container: ", err.Error())
		}

	} else {
		log.Fatal("Failed to create skupper-proxy-controller container")
	}

}

func expose(targetName string, options skupperservice.ExposeOptions) {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	// check that init has fun first by inspecting skupper router container
	_, err := dd.InspectContainer("skupper-router")
	if err != nil {
		if dockerapi.IsErrNotFound(err) {
			log.Fatal("Router container does not exist (need init?): " + err.Error())
		} else {
			log.Fatal("Error retrieving router container: " + err.Error())
		}
		return
	}

	// check that a service with that name already has been attached to the VAN
	_, err = ioutil.ReadFile(skupperServicePath + options.Address)
	if err == nil {
		// TODO: Deal with update case , read in json file, decode and update
		fmt.Printf("Expose target name %s already exists\n", targetName)
		return
	}

	if targetName == options.Address {
		fmt.Println("The exposed address and container target name must be different")
		return
	} else {
		// TODO: container exists but exited, not running? restart it?
		_, err := dd.InspectContainer(targetName)

		if err != nil {
			if dockerapi.IsErrNotFound(err) {
				log.Fatal("Target container does not exist: " + err.Error())
			} else {
				log.Fatal("Error retrieving service target container: " + err.Error())
			}
			return
		}
	}

	serviceTarget := skupperservice.ServiceTarget{
		Name:       targetName,
		Selector:   "",
		TargetPort: options.TargetPort,
	}

	serviceDef := skupperservice.Service{
		Address:  options.Address,
		Protocol: options.Protocol,
		Port:     options.Port,
		Targets: []skupperservice.ServiceTarget{
			serviceTarget,
		},
	}

	encoded, err := json.Marshal(serviceDef)

	if err != nil {
		log.Fatal("Failed to create json for service definition: ", err.Error())
		return
	} else {
		if err = ioutil.WriteFile(skupperServicePath+options.Address, encoded, 0755); err != nil {
			log.Fatal("Failed to write services file: ", err.Error())
		}
	}

	return
}

func unexpose(address string) bool {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	// check that init has fun first by inspecting skupper router container
	_, err := dd.InspectContainer("skupper-router")
	if err != nil {
		if dockerapi.IsErrNotFound(err) {
			log.Fatal("Router container does not exist (need init?): " + err.Error())
		} else {
			log.Fatal("Error retrieving router container: " + err.Error())
		}
		return false
	}

	// check that a service with that name already has been attached to the VAN
	_, err = ioutil.ReadFile(skupperServicePath + address)
	if err != nil {
		log.Printf("Service address %s does not exist\n", address)
		return false
	} else {
		// remove the service definition file
		os.Remove(skupperServicePath + address)
	}
	return true
}

func countServiceDefinitions() int {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)
    
	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/component")

	containers, err := dd.ListContainers(dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	})
	if err != nil {
		return 0
	}
    if err == nil {
        count := 0
    	for _, container := range containers {
	    	if value, ok := container.Labels["skupper.io/last-applied"]; ok {
		    	service := skupperservice.Service{}
			    err = json.Unmarshal([]byte(value), &service)
                if err != nil {
                    fmt.Printf("Invalid service definition %s: %s", container.Names[0], err)
                    fmt.Println()
                } else {
                    count = count + 1
                }
            }
        }
        return count
    } else {
        fmt.Println("Could not retrieve service container list: ", err.Error())
        return 0
    }

}

func listServiceDefinitions() {
	dd := libdocker.ConnectToDockerOrDie("", 0, 10*time.Second)

	filters := dockerfilters.NewArgs()
	filters.Add("label", "skupper.io/component")

	containers, err := dd.ListContainers(dockertypes.ContainerListOptions{
		Filters: filters,
		All:     true,
	})
	if err != nil {
		log.Fatal("Failed to list proxy containers: ", err.Error())
	}

	for _, container := range containers {
		if value, ok := container.Labels["skupper.io/last-applied"]; ok {
			service := skupperservice.Service{}
			err = json.Unmarshal([]byte(value), &service)
			if err != nil {
				log.Fatal("Failed to parse json for service definition", err.Error())
			} else if len(service.Targets) == 0 {
				fmt.Printf("    %s (%s port %d)", service.Address, service.Protocol, service.Port)
				fmt.Println()
			} else {
				fmt.Printf("    %s (%s port %d) with targets", service.Address, service.Protocol, service.Port)
				fmt.Println()
				for _, t := range service.Targets {
					var name string
					if t.Name != "" {
						name = fmt.Sprintf("name=%s", t.Name)
					}
					fmt.Printf("      => %s %s", t.Selector, name)
					fmt.Println()
				}
			}
		}
	}
}

func requiredArg(name string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("%s must be specified", name)
		}
		if len(args) > 1 {
			return fmt.Errorf("illegal argument: %s", args[1])
		}
		return nil
	}
}

func main() {
	var skupperName string
	var isEdge bool
	var enableProxyController bool
	var enableServiceSync bool
	var enableRouterConsole bool
	var routerConsoleAuthMode string
	var routerConsoleUser string
	var routerConsolePassword string

	var cmdInit = &cobra.Command{
		Use:   "init",
		Short: "Initialize skupper docker installation",
		Long:  `init will setup a router and other supporting objects to provide a functional skupper installation that can be connected to a VAN deployment`,
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			router := Router{
				Name: skupperName,
				Mode: RouterModeInterior,
			}
			if isEdge {
				router.Mode = RouterModeEdge
			} else {
				router.Mode = RouterModeInterior
			}
			if enableRouterConsole {
				if routerConsoleAuthMode == string(ConsoleAuthModeInternal) || routerConsoleAuthMode == "" {
					router.Console = ConsoleAuthModeInternal
					router.ConsoleUser = routerConsoleUser
					router.ConsolePassword = routerConsolePassword
					if router.ConsoleUser == "" {
						router.ConsoleUser = "admin"
					}
					if router.ConsolePassword == "" {
						router.ConsolePassword = randomId(10)
					}
				} else {
					if routerConsoleUser != "" {
						log.Fatal("--router-console-user only valid when --router-console-auth=internal")
					}
					if routerConsolePassword != "" {
						log.Fatal("--router-console-password only valid when --router-console-auth=internal")
					}
					if routerConsoleAuthMode == string(ConsoleAuthModeUnsecured) {
						router.Console = ConsoleAuthModeUnsecured
					} else {
						log.Fatal("Unrecognised router console authentication mode: ", routerConsoleAuthMode)
					}
				}
			}
			skupperInit(&router)
			fmt.Println("Skupper is now installed.  Use 'skupper status' to get more information.")
		},
	}

	cmdInit.Flags().StringVarP(&skupperName, "id", "", "", "Provide a specific identity for the skupper installation")
	cmdInit.Flags().BoolVarP(&isEdge, "edge", "", false, "Configure as an edge")
	cmdInit.Flags().BoolVarP(&enableProxyController, "enable-proxy-controller", "", false, "Setup the proxy controller as well as the router")
	cmdInit.Flags().BoolVarP(&enableServiceSync, "enable-service-sync", "", true, "Configure proxy controller to particiapte in service sync (not relevant if --enable-proxy-controller is false)")
	cmdInit.Flags().BoolVarP(&enableRouterConsole, "enable-router-console", "", false, "Enable router console")
	cmdInit.Flags().StringVarP(&routerConsoleAuthMode, "router-console-auth", "", "", "Authentication mode for router console. One of: 'internal', 'unsecured'")
	cmdInit.Flags().StringVarP(&routerConsoleUser, "router-console-user", "", "", "Router console user. Valid only when --router-console-auth=internal")
	cmdInit.Flags().StringVarP(&routerConsolePassword, "router-console-password", "", "", "Router console user. Valid only when --router-console-auth=internal")

	var cmdDelete = &cobra.Command{
		Use:   "delete",
		Short: "Delete skupper installation",
		Long:  `delete will delete any skupper related objects`,
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			delete()
		},
	}

	var clientIdentity string
	var cmdConnectionToken = &cobra.Command{
		Use:   "connection-token <output-file>",
		Short: "Create a connection token file with which another skupper installation can connect to this one",
		Args:  requiredArg("output-file"),
		Run: func(cmd *cobra.Command, args []string) {
			generateConnectSecret(clientIdentity, args[0])
		},
	}
	cmdConnectionToken.Flags().StringVarP(&clientIdentity, "client-identity", "i", "skupper", "Provide a specific identity as which connecting skupper installation will be authenticated")

	var listConnectors bool
	var cmdStatus = &cobra.Command{
		Use:   "status",
		Short: "Report status of skupper installation",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			status(listConnectors)
		},
	}
	cmdStatus.Flags().BoolVarP(&listConnectors, "list-connectors", "", false, "List configured outgoing connections")

	var connectionName string
	var cost int
	var cmdConnect = &cobra.Command{
		Use:   "connect <connection-token-file>",
		Short: "Connect this skupper installation to that which issued the specified connectionToken",
		Args:  requiredArg("connection-token"),
		Run: func(cmd *cobra.Command, args []string) {
			connect(args[0], connectionName, cost)
		},
	}
	cmdConnect.Flags().StringVarP(&connectionName, "connection-name", "", "", "Provide a specific name for the connection (used when removing it with disconnect)")
	cmdConnect.Flags().IntVarP(&cost, "cost", "", 1, "Specify a cost for this connection.")

	var cmdDisconnect = &cobra.Command{
		Use:   "disconnect <name>",
		Short: "Remove specified connection",
		Args:  requiredArg("connection name"),
		Run: func(cmd *cobra.Command, args []string) {
			disconnect(args[0])
		},
	}

	var waitFor int
	var cmdCheckConnection = &cobra.Command{
		Use:   "check-connection all|<connection-name>",
		Short: "Check whether a connection to another skupper site is active",
		Args:  requiredArg("connection name"),
		Run: func(cmd *cobra.Command, args []string) {
			result := checkConnection(args[0])
			for i := 0; !result && i < waitFor; i++ {
				time.Sleep(time.Second)
				result = checkConnection(args[0])
			}
			if !result {
				os.Exit(-1)
			}
		},
	}
	cmdCheckConnection.Flags().IntVar(&waitFor, "wait", 0, "Number of seconds to wait for connection(s) to become active")

	var cmdVersion = &cobra.Command{
		Use:   "version",
		Short: "Report version of skupper cli and services",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("client version           %s\n", version)
		},
	}

	exposeOptions := skupperservice.ExposeOptions{}
	var cmdExpose = &cobra.Command{
		Use:   "expose <name>",
		Short: "Expose a skupper address and optionally a local target to the skupper network",
		Args:  requiredArg("address"),
		Run: func(cmd *cobra.Command, args []string) {
			expose(args[0], exposeOptions)
		},
	}
	cmdExpose.Flags().StringVar(&(exposeOptions.Protocol), "protocol", "tcp", "Protocol to proxy (tcp, http or http2)")
	cmdExpose.Flags().StringVar(&(exposeOptions.Address), "address", "", "Skupper address to expose as")
	cmdExpose.Flags().IntVar(&(exposeOptions.Port), "port", 0, "Port to expose on")
	cmdExpose.Flags().IntVar(&(exposeOptions.TargetPort), "target-port", 0, "Port to target on pods")
	cmdExpose.Flags().BoolVar(&(exposeOptions.Headless), "headless", false, "Expose through headless service (valid only for statefulset target)")

	var cmdUnexpose = &cobra.Command{
		Use:   "unexpose <name>",
		Short: "Unexpose container previously exposed via skupper address",
		Args:  requiredArg("address"),
		Run: func(cmd *cobra.Command, args []string) {
			unexpose(args[0])
			//if unexpose(aunexposeAddress) {
			//	fmt.Printf("Address %s detached from application network\n", unexposeAddress)
			//}
		},
	}

	var cmdListExposed = &cobra.Command{
		Use:   "list-exposed",
		Short: "List services exposed over the skupper network",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			listServiceDefinitions()
		},
	}

	var rootCmd = &cobra.Command{Use: "skupper"}
	rootCmd.Version = version
	rootCmd.AddCommand(cmdInit, cmdDelete, cmdConnectionToken, cmdConnect, cmdDisconnect, cmdCheckConnection, cmdStatus, cmdVersion, cmdExpose, cmdUnexpose, cmdListExposed)
	rootCmd.Execute()
}
