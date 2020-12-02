package qdr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	dockertypes "github.com/docker/docker/api/types"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

type RouterConfig struct {
	Metadata    RouterMetadata
	SslProfiles map[string]SslProfile
	Listeners   map[string]Listener
	Connectors  map[string]Connector
	Addresses   map[string]Address
	Bridges     BridgeConfig
}

type TcpEndpointMap map[string]TcpEndpoint
type HttpEndpointMap map[string]HttpEndpoint

type BridgeConfig struct {
	TcpListeners   TcpEndpointMap
	TcpConnectors  TcpEndpointMap
	HttpListeners  HttpEndpointMap
	HttpConnectors HttpEndpointMap
}

func InitialConfig(id string, metadata string, edge bool) RouterConfig {
	config := RouterConfig{
		Metadata: RouterMetadata{
			Id:       id,
			Metadata: metadata,
		},
		Addresses:   map[string]Address{},
		SslProfiles: map[string]SslProfile{},
		Listeners:   map[string]Listener{},
		Connectors:  map[string]Connector{},
		Bridges: BridgeConfig{
			TcpListeners:   map[string]TcpEndpoint{},
			TcpConnectors:  map[string]TcpEndpoint{},
			HttpListeners:  map[string]HttpEndpoint{},
			HttpConnectors: map[string]HttpEndpoint{},
		},
	}
	if edge {
		config.Metadata.Mode = ModeEdge
	} else {
		config.Metadata.Mode = ModeInterior
	}
	return config
}

func NewBridgeConfig() BridgeConfig {
	return BridgeConfig{
		TcpListeners:   map[string]TcpEndpoint{},
		TcpConnectors:  map[string]TcpEndpoint{},
		HttpListeners:  map[string]HttpEndpoint{},
		HttpConnectors: map[string]HttpEndpoint{},
	}
}
func IsInterior(qdr *dockertypes.ContainerJSON) bool {
	config := docker.FindEnvVar(qdr.Config.Env, types.TransportEnvConfig)
	if config == "" {
		return false
	} else {
		match, _ := regexp.MatchString("mode:[ ]+interior", config)
		return match
	}
}

func GetTransportMode(qdr *dockertypes.ContainerJSON) types.TransportMode {
	if IsInterior(qdr) {
		return types.TransportModeInterior
	} else {
		return types.TransportModeEdge
	}
}

func (r *RouterConfig) AddListener(l Listener) {
	if l.Name == "" {
		l.Name = fmt.Sprintf("%s@%d", l.Host, l.Port)
	}
	r.Listeners[l.Name] = l
}

func (r *RouterConfig) AddConnector(c Connector) {
	r.Connectors[c.Name] = c
}

func (r *RouterConfig) RemoveConnector(name string) bool {
	_, ok := r.Connectors[name]
	if ok {
		delete(r.Connectors, name)
		return true
	} else {
		return false
	}
}

func (r *RouterConfig) IsEdge() bool {
	return r.Metadata.Mode == ModeEdge
}

func (r *RouterConfig) AddSslProfile(s SslProfile) {
	if s.CertFile == "" && s.CaCertFile == "" && s.PrivateKeyFile == "" {
		s.CertFile = fmt.Sprintf("/etc/qpid-dispatch-certs/%s/tls.crt", s.Name)
		s.PrivateKeyFile = fmt.Sprintf("/etc/qpid-dispatch-certs/%s/tls.key", s.Name)
		s.CaCertFile = fmt.Sprintf("/etc/qpid-dispatch-certs/%s/ca.crt", s.Name)
	}
	r.SslProfiles[s.Name] = s
}

func (r *RouterConfig) AddConnSslProfile(s SslProfile) {
	dir := strings.TrimSuffix(s.Name, "-profile")
	if s.CertFile == "" && s.CaCertFile == "" && s.PrivateKeyFile == "" {
		s.CertFile = fmt.Sprintf("/etc/qpid-dispatch/connections/%s/tls.crt", dir)
		s.PrivateKeyFile = fmt.Sprintf("/etc/qpid-dispatch/connections/%s/tls.key", dir)
		s.CaCertFile = fmt.Sprintf("/etc/qpid-dispatch/connections/%s/ca.crt", dir)
	}
	r.SslProfiles[s.Name] = s
}

func (r *RouterConfig) RemoveConnSslProfile(name string) bool {
	profileName := name + "-profile"
	_, ok := r.SslProfiles[profileName]
	if ok {
		delete(r.SslProfiles, profileName)
		return true
	} else {
		return false
	}
}

func (r *RouterConfig) AddAddress(a Address) {
	r.Addresses[a.Prefix] = a
}

func (r *RouterConfig) AddTcpConnector(e TcpEndpoint) {
	r.Bridges.AddTcpConnector(e)
}

func (r *RouterConfig) AddTcpListener(e TcpEndpoint) {
	r.Bridges.AddTcpListener(e)
}

func (r *RouterConfig) AddHttpConnector(e HttpEndpoint) {
	r.Bridges.AddHttpConnector(e)
}

func (r *RouterConfig) AddHttpListener(e HttpEndpoint) {
	r.Bridges.AddHttpListener(e)
}

func (r *RouterConfig) UpdateBridgeConfig(desired BridgeConfig) bool {
	if reflect.DeepEqual(r.Bridges, desired) {
		return false
	} else {
		r.Bridges = desired
		return true
	}
}

func (bc *BridgeConfig) AddTcpConnector(e TcpEndpoint) {
	bc.TcpConnectors[e.Name] = e
}

func (bc *BridgeConfig) AddTcpListener(e TcpEndpoint) {
	bc.TcpListeners[e.Name] = e
}

func (bc *BridgeConfig) AddHttpConnector(e HttpEndpoint) {
	bc.HttpConnectors[e.Name] = e
}

func (bc *BridgeConfig) AddHttpListener(e HttpEndpoint) {
	bc.HttpListeners[e.Name] = e
}

type Role string

const (
	RoleInterRouter Role = "inter-router"
	RoleEdge             = "edge"
)

type Mode string

const (
	ModeInterior Mode = "interior"
	ModeEdge          = "edge"
)

type RouterMetadata struct {
	Id       string `json:"id,omitempty"`
	Mode     Mode   `json:"mode,omitempty"`
	Metadata string `json:"metadata,omitempty"`
}

type SslProfile struct {
	Name           string `json:"name,omitempty"`
	CertFile       string `json:"certFile,omitempty"`
	PrivateKeyFile string `json:"privateKeyFile,omitempty"`
	CaCertFile     string `json:"caCertFile,omitempty"`
}

type Listener struct {
	Name             string `json:"name,omitempty"`
	Role             Role   `json:"role,omitempty"`
	Host             string `json:"host,omitempty"`
	Port             int32  `json:"port"`
	RouteContainer   bool   `json:"routeContainer,omitempty"`
	Http             bool   `json:"http,omitempty"`
	Cost             int32  `json:"cost,omitempty"`
	SslProfile       string `json:"sslProfile,omitempty"`
	SaslMechanisms   string `json:"saslMechanisms,omitempty"`
	AuthenticatePeer bool   `json:"authenticatePeer,omitempty"`
	LinkCapacity     int32  `json:"linkCapacity,omitempty"`
	HttpRootDir      string `json:"httpRootDir,omitempty"`
	Websockets       bool   `json:"websockets,omitempty"`
	Healthz          bool   `json:"healthz,omitempty"`
	Metrics          bool   `json:"metrics,omitempty"`
}

type Connector struct {
	Name           string `json:"name,omitempty"`
	Role           Role   `json:"role,omitempty"`
	Host           string `json:"host"`
	Port           string `json:"port"`
	RouteContainer bool   `json:"routeContainer,omitempty"`
	Cost           int32  `json:"cost,omitempty"`
	VerifyHostname bool   `json:"verifyHostname,omitempty"`
	SslProfile     string `json:"sslProfile,omitempty"`
	LinkCapacity   int32  `json:"linkCapacity,omitempty"`
}

type Distribution string

const (
	DistributionBalanced  Distribution = "balanced"
	DistributionMulticast              = "multicast"
	DistributionClosest                = "closest"
)

type Address struct {
	Prefix       string `json:"prefix,omitempty"`
	Distribution string `json:"distribution,omitempty"`
}

type TcpEndpoint struct {
	Name    string `json:"name,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    string `json:"port,omitempty"`
	Address string `json:"address,omitempty"`
	SiteId  string `json:"siteId,omitempty"`
}

type HttpEndpoint struct {
	Name            string `json:"name,omitempty"`
	Host            string `json:"host,omitempty"`
	Port            string `json:"port,omitempty"`
	Address         string `json:"address,omitempty"`
	SiteId          string `json:"siteId,omitempty"`
	ProtocolVersion string `json:"protocolVersion,omitempty"`
	Aggregation     string `json:"aggregation,omitempty"`
	EventChannel    bool   `json:"eventChannel,omitempty"`
	HostOverride    string `json:"hostOverride,omitempty"`
}

func convert(from interface{}, to interface{}) error {
	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, to)
	if err != nil {
		return err
	}
	return nil
}

func UnmarshalRouterConfig(config string) (RouterConfig, error) {
	result := RouterConfig{
		Metadata:    RouterMetadata{},
		Addresses:   map[string]Address{},
		SslProfiles: map[string]SslProfile{},
		Listeners:   map[string]Listener{},
		Connectors:  map[string]Connector{},
		Bridges: BridgeConfig{
			TcpListeners:   map[string]TcpEndpoint{},
			TcpConnectors:  map[string]TcpEndpoint{},
			HttpListeners:  map[string]HttpEndpoint{},
			HttpConnectors: map[string]HttpEndpoint{},
		},
	}
	var obj interface{}
	err := json.Unmarshal([]byte(config), &obj)
	if err != nil {
		return result, err
	}
	elements, ok := obj.([]interface{})
	if !ok {
		return result, fmt.Errorf("Invalid JSON for router configuration, expected array at top level got %#v", obj)
	}
	for _, e := range elements {
		element, ok := e.([]interface{})
		if !ok || len(element) != 2 {
			return result, fmt.Errorf("Invalid JSON for router configuration, expected array with type and value got %#v", e)
		}
		entityType, ok := element[0].(string)
		if !ok {
			return result, fmt.Errorf("Invalid JSON for router configuration, expected entity type as string got %#v", element[0])
		}
		switch entityType {
		case "router":
			metadata := RouterMetadata{}
			err = convert(element[1], &metadata)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Metadata = metadata
		case "address":
			address := Address{}
			err = convert(element[1], &address)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Addresses[address.Prefix] = address
		case "connector":
			connector := Connector{}
			err = convert(element[1], &connector)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Connectors[connector.Name] = connector
		case "listener":
			listener := Listener{}
			err = convert(element[1], &listener)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Listeners[listener.Name] = listener
		case "sslProfile":
			sslProfile := SslProfile{}
			err = convert(element[1], &sslProfile)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.SslProfiles[sslProfile.Name] = sslProfile
		case "tcpConnector":
			connector := TcpEndpoint{}
			err = convert(element[1], &connector)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Bridges.TcpConnectors[connector.Name] = connector
		case "tcpListener":
			listener := TcpEndpoint{}
			err = convert(element[1], &listener)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Bridges.TcpListeners[listener.Name] = listener
		case "httpConnector":
			connector := HttpEndpoint{}
			err = convert(element[1], &connector)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Bridges.HttpConnectors[connector.Name] = connector
		case "httpListener":
			listener := HttpEndpoint{}
			err = convert(element[1], &listener)
			if err != nil {
				return result, fmt.Errorf("Invalid %s element got %#v", entityType, element[1])
			}
			result.Bridges.HttpListeners[listener.Name] = listener
		default:
		}
	}
	return result, nil
}

func MarshalRouterConfig(config RouterConfig) (string, error) {
	elements := [][]interface{}{}
	tuple := []interface{}{
		"router",
		config.Metadata,
	}
	elements = append(elements, tuple)
	for _, e := range config.SslProfiles {
		tuple := []interface{}{
			"sslProfile",
			e,
		}
		elements = append(elements, tuple)
	}
	for _, e := range config.Connectors {
		tuple := []interface{}{
			"connector",
			e,
		}
		elements = append(elements, tuple)
	}
	for _, e := range config.Listeners {
		tuple := []interface{}{
			"listener",
			e,
		}
		elements = append(elements, tuple)
	}
	for _, e := range config.Addresses {
		tuple := []interface{}{
			"address",
			e,
		}
		elements = append(elements, tuple)
	}
	for _, e := range config.Bridges.TcpConnectors {
		tuple := []interface{}{
			"tcpConnector",
			e,
		}
		elements = append(elements, tuple)
	}
	for _, e := range config.Bridges.TcpListeners {
		tuple := []interface{}{
			"tcpListener",
			e,
		}
		elements = append(elements, tuple)
	}
	for _, e := range config.Bridges.HttpConnectors {
		tuple := []interface{}{
			"httpConnector",
			e,
		}
		elements = append(elements, tuple)
	}
	for _, e := range config.Bridges.HttpListeners {
		tuple := []interface{}{
			"httpListener",
			e,
		}
		elements = append(elements, tuple)
	}
	data, err := json.MarshalIndent(elements, "", "    ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func GetRouterConfigForProxy(definition types.ServiceInterface, siteId string) (string, error) {
	config := InitialConfig("$HOSTNAME", siteId, true)
	//add edge-connector
	config.AddSslProfile(SslProfile{
		Name: types.InterRouterProfile,
	})
	config.AddConnector(Connector{
		Name:       "uplink",
		SslProfile: types.InterRouterProfile,
		Host:       "skupper-router",
		Port:       strconv.Itoa(int(types.EdgeListenerPort)),
		Role:       RoleEdge,
	})
	config.AddListener(Listener{
		Name: "amqp",
		Host: "localhost",
		Port: 5672,
	})
	port := definition.Port
	if definition.Origin == "" {
		host := definition.Address
		switch definition.Protocol {
		case "tcp":
			config.AddTcpListener(TcpEndpoint{
				Name:    "ingress",
				Host:    "0.0.0.0",
				Port:    strconv.Itoa(port),
				Address: definition.Address,
				SiteId:  siteId,
			})
			for _, t := range definition.Targets {
				tport := definition.Port
				if t.TargetPort != 0 {
					tport = t.TargetPort
				}
				if t.Selector == "internal.skupper.io/container" {
					config.AddTcpConnector(TcpEndpoint{
						Name:    "egress-" + t.Name,
						Host:    t.Name,
						Port:    strconv.Itoa(tport),
						Address: definition.Address,
						SiteId:  siteId,
					})
				} else if t.Selector == "internal.skupper.io/host-service" {
					thost := strings.Split(t.Name, ":")
					config.AddTcpConnector(TcpEndpoint{
						Name:    "egress-" + thost[0],
						Host:    thost[0],
						Port:    strconv.Itoa(tport),
						Address: definition.Address,
						SiteId:  siteId,
					})
				}
			}
		case "http":
			config.AddHttpListener(HttpEndpoint{
				Name:    "ingress",
				Host:    host,
				Port:    strconv.Itoa(port),
				Address: definition.Address,
				SiteId:  siteId,
			})
			for _, t := range definition.Targets {
				tport := definition.Port
				if t.TargetPort != 0 {
					tport = t.TargetPort
				}
				if t.Selector == "internal.skupper.io/container" {
					config.AddHttpConnector(HttpEndpoint{
						Name:    "egress-" + t.Name,
						Host:    t.Name,
						Port:    strconv.Itoa(tport),
						Address: definition.Address,
						SiteId:  siteId,
					})
				} else if t.Selector == "internal.skupper.io/host-service" {
					thost := strings.Split(t.Name, ":")
					config.AddHttpConnector(HttpEndpoint{
						Name:    "egress-" + thost[0],
						Host:    thost[0],
						Port:    strconv.Itoa(tport),
						Address: definition.Address,
						SiteId:  siteId,
					})
				}
			}
		case "http2":
			config.AddHttpListener(HttpEndpoint{
				Name:            "ingress",
				Host:            host,
				Port:            strconv.Itoa(port),
				Address:         definition.Address,
				ProtocolVersion: "HTTP/2.0",
				SiteId:          siteId,
			})
			for _, t := range definition.Targets {
				tport := definition.Port
				if t.TargetPort != 0 {
					tport = t.TargetPort
				}
				if t.Selector == "internal.skupper.io/container" {
					config.AddHttpConnector(HttpEndpoint{
						Name:            "egress-" + t.Name,
						Host:            t.Name,
						Port:            strconv.Itoa(tport),
						Address:         definition.Address,
						ProtocolVersion: "HTTP/2.0",
						SiteId:          siteId,
					})
				} else if t.Selector == "internal.skupper.io/host-service" {
					thost := strings.Split(t.Name, ":")
					config.AddHttpConnector(HttpEndpoint{
						Name:            "egress-" + thost[0],
						Host:            thost[0],
						Port:            strconv.Itoa(tport),
						Address:         definition.Address,
						ProtocolVersion: "HTTP/2.0",
						SiteId:          siteId,
					})
				}
			}
		default:
		}
	} else {
		host := "0.0.0.0"
		//in all other sites, just have ingress bindings
		switch definition.Protocol {
		case "tcp":
			config.AddTcpListener(TcpEndpoint{
				Name:    "ingress",
				Host:    host,
				Port:    strconv.Itoa(port),
				Address: definition.Address,
				SiteId:  siteId,
			})
		case "http":
			config.AddHttpListener(HttpEndpoint{
				Name:    "ingress",
				Host:    host,
				Port:    strconv.Itoa(port),
				Address: definition.Address,
				SiteId:  siteId,
			})
		case "http2":
			config.AddHttpListener(HttpEndpoint{
				Name:            "ingress",
				Host:            host,
				Port:            strconv.Itoa(port),
				Address:         definition.Address,
				ProtocolVersion: "HTTP/2.0",
				SiteId:          siteId,
			})
		default:
		}
	}
	return MarshalRouterConfig(config)
}

func (r *RouterConfig) WriteToConfigFile(configFile string) error {
	marshalled, err := MarshalRouterConfig(*r)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(configFile, []byte(marshalled), 0755)
}

func GetRouterConfigFromFile(name string) (*RouterConfig, error) {
	if name == "" {
		return nil, nil
	}

	config, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}
	routerConfig, err := UnmarshalRouterConfig(string(config))
	if err != nil {
		return nil, err
	}
	return &routerConfig, nil

}
