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

package types

import (
	"github.com/docker/go-connections/nat"
)

const (
	DefaultVanName    string = "skupper"
	DefaultBridgeName string = "skupper0"
	HostPath          string = "/tmp/skupper"
	CertPath                 = HostPath + "/qpid-dispatch-certs/"
	ConnPath                 = HostPath + "/connections/"
	ConfigPath               = HostPath + "/config/"
	ConsoleUsersPath         = HostPath + "/console-users/"
	SaslConfigPath           = HostPath + "/sasl-config/"
	ServicePath              = HostPath + "/services/"
	SitePath                 = HostPath + "/sites/"
)

// TransportMode describes how a qdr is intended to be deployed, either interior or edge
type TransportMode string

const (
	// TransportModeInterior means the qdr will participate in inter-router protocol exchanges
	TransportModeInterior TransportMode = "interior"
	// TransportModeEdge means that the qdr will connect to interior routers for network access
	TransportModeEdge = "edge"
)

// Transport constants
const (
	TransportDeploymentName string = "skupper-router"
	TransportComponentName  string = "router"
	//	DefaultTransportImage   string = "quay.io/interconnectedcloud/qdrouterd"
	DefaultTransportImage  string = "quay.io/gordons/qdrouterd:0.4.0-alpha-1"
	TransportContainerName string = "router"
	TransportLivenessPort  int32  = 9090
	TransportEnvConfig     string = "QDROUTERD_CONF"
	TransportSaslConfig    string = "skupper-sasl-config"
	TransportNetworkName   string = "skupper-network"
	TransportConfigFile    string = "qdrouterd.json"
)

var TransportPrometheusAnnotations = map[string]string{
	"prometheus.io/port":   "9090",
	"prometheus.io/scrape": "true",
}

// Controller constants
const (
	ControllerDeploymentName string = "skupper-service-controller"
	ControllerComponentName  string = "controller"
	DefaultControllerImage   string = "quay.io/skupper/skupper-docker-controller"
	ControllerContainerName  string = "service-controller"
	ControllerConfigPath     string = "/etc/messaging/"
)

// Skupper qualifiers
const (
	BaseQualifier    string = "skupper.io"
	TokenGeneratedBy string = BaseQualifier + "/generated-by"
	TokenCost        string = BaseQualifier + "/cost"
)

// Console constants
const (
	ConsolePortName                        string = "console"
	ConsoleServiceName                     string = "skupper-console"
	ConsoleDefaultServicePort              int32  = 8080
	ConsoleDefaultServiceTargetPort        int32  = 8080
	ConsoleOpenShiftServicePort            int32  = 8888
	ConsoleOpenShiftOauthServicePort       int32  = 443
	ConsoleOpenShiftOuathServiceTargetPort int32  = 8443
	ConsoleOpenShiftServingCerts           string = "skupper-proxy-certs"
)

type ConsoleAuthMode string

const (
	ConsoleAuthModeInternal  ConsoleAuthMode = "internal"
	ConsoleAuthModeUnsecured                 = "unsecured"
)

// Router constants
const (
	AmqpDefaultPort         int32  = 5672
	AmqpsDefaultPort        int32  = 5671
	EdgeRole                string = "edge"
	EdgeRouteName           string = "skupper-edge"
	EdgeListenerPort        int32  = 45671
	InterRouterRole         string = "inter-router"
	InterRouterListenerPort int32  = 55671
	InterRouterRouteName    string = "skupper-inter-router"
	InterRouterProfile      string = "skupper-internal"
)

// Controller Service Interface constants
const (
	ServiceSyncAddress = "mc/$skupper-service-sync"
	ServiceDefsFile    = ServicePath + "/skupper-services"
)

// TODO: what is possiblity of using types from skupper itself (e.g. no namespace for docker
// or we change the name to endpoint, etc.
// RouterSpec is the specification of VAN network with router, controller and assembly
type RouterSpec struct {
	Name           string          `json:"name,omitempty"`
	AuthMode       ConsoleAuthMode `json:"authMode,omitempty"`
	Transport      DeploymentSpec  `json:"transport,omitempty"`
	Controller     DeploymentSpec  `json:"controller,omitempty"`
	RouterConfig   string          `json:"routerConfig,omitempty"`
	Users          []User          `json:"users,omitempty"`
	CertAuthoritys []CertAuthority `json:"certAuthoritys,omitempty"`
	Credentials    []Credential    `json:"credentials,omitempty"`
}

// DeploymentSpec for the VAN router or controller components to run within a cluster
type DeploymentSpec struct {
	Image        string            `json:"image,omitempty"`
	LivenessPort int32             `json:"livenessPort,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	EnvVar       []string          `json:"envVar,omitempty"`
	Ports        nat.PortSet       `json:"ports,omitempty"`
	Volumes      []string          `json:"volumes,omitempty"`
	Mounts       map[string]string `json:"mounts,omitempty"`
}

type ConnectorRole string

const (
	ConnectorRoleInterRouter ConnectorRole = "inter-router"
	ConnectorRoleEdge                      = "edge"
)

type Connector struct {
	Name           string `json:"name,omitempty"`
	Role           string `json:"role,omitempty"`
	Host           string `json:"host"`
	Port           string `json:"port"`
	RouteContainer bool   `json:"routeContainer,omitempty"`
	Cost           int32  `json:"cost,omitempty"`
	VerifyHostname bool   `json:"verifyHostname,omitempty"`
	SslProfile     string `json:"sslProfile,omitempty"`
	LinkCapacity   int32  `json:"linkCapacity,omitempty"`
}

type Credential struct {
	CA          string
	Name        string
	Subject     string
	Hosts       []string
	ConnectJson bool
	Post        bool
	Data        map[string][]byte
}

type CertAuthority struct {
	Name string
}

type User struct {
	Name     string
	Password string
}

type TransportConnectedSites struct {
	Direct   int
	Indirect int
	Total    int
	Warnings []string
}

type ServiceInterface struct {
	Address      string                   `json:"address"`
	Protocol     string                   `json:"protocol"`
	Port         int                      `json:"port"`
	EventChannel bool                     `json:"eventchannel,omitempty"`
	Aggregate    string                   `json:"aggregate,omitempty"`
	Headless     *Headless                `json:"headless,omitempty"`
	Targets      []ServiceInterfaceTarget `json:"targets"`
	Origin       string                   `json:"origin,omitempty"`
	Alias        string                   `json:"alias,omitempty"`
}

type ServiceInterfaceTarget struct {
	Name       string `json:"name,omitempty"`
	Selector   string `json:"selector"`
	TargetPort int    `json:"targetPort,omitempty"`
	Service    string `json:"service,omitempty"`
}

type Headless struct {
	Name       string `json:"name"`
	Size       int    `json:"size"`
	TargetPort int    `json:"targetPort,omitempty"`
}

type ByServiceInterfaceAddress []ServiceInterface

func (a ByServiceInterfaceAddress) Len() int {
	return len(a)
}

func (a ByServiceInterfaceAddress) Less(i, j int) bool {
	return a[i].Address > a[i].Address
}

func (a ByServiceInterfaceAddress) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
