package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"

	dockertypes "github.com/docker/docker/api/types"

	"github.com/docker/go-connections/nat"

	"github.com/skupperproject/skupper/pkg/certs"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
	"github.com/skupperproject/skupper-docker/pkg/utils"
	"github.com/skupperproject/skupper-docker/pkg/utils/configs"
)

func getCertData(name string) (certs.CertificateData, error) {
	certData := certs.CertificateData{}
	certPath := types.GetSkupperPath(types.CertsPath) + "/" + name

	files, err := ioutil.ReadDir(certPath)
	if err == nil {
		for _, f := range files {
			dataString, err := ioutil.ReadFile(certPath + "/" + f.Name())
			if err == nil {
				certData[f.Name()] = []byte(dataString)
			} else {
				return certData, fmt.Errorf("Failed to read certificat data: %w", err)
			}
		}
	}
	return certData, err
}

func generateCredentials(ca string, name string, subject string, hosts []string, includeConnectJson bool) error {
	caData, _ := getCertData(ca)
	certData := certs.GenerateCertificateData(name, subject, strings.Join(hosts, ","), caData)

	for k, v := range certData {
		if err := ioutil.WriteFile(types.GetSkupperPath(types.CertsPath)+"/"+name+"/"+k, v, 0755); err != nil {
			return fmt.Errorf("Failed to write certificate file: %w", err)
		}
	}

	if includeConnectJson {
		certData["connect.json"] = []byte(configs.ConnectJSON())
		if err := ioutil.WriteFile(types.GetSkupperPath(types.CertsPath)+"/"+name+"/connect.json", []byte(configs.ConnectJSON()), 0755); err != nil {
			return fmt.Errorf("Failed to write connect file: %w", err)
		}
	}

	return nil
}

func ensureCA(name string) (certs.CertificateData, error) {

	// check if existing by looking at path/dir, if not create dir to persist
	caData := certs.GenerateCACertificateData(name, name)

	if err := os.Mkdir(types.GetSkupperPath(types.CertsPath)+"/"+name, 0755); err != nil {
		return nil, fmt.Errorf("Failed to create certificate directory: %w", err)
	}

	for k, v := range caData {
		if err := ioutil.WriteFile(types.GetSkupperPath(types.CertsPath)+"/"+name+"/"+k, v, 0755); err != nil {
			return nil, fmt.Errorf("Failed to write CA certificate file: %w", err)
		}
	}

	return caData, nil
}

func (cli *VanClient) GetRouterSpecFromOpts(options types.SiteConfigSpec, siteId string) (*types.RouterSpec, error) {
	van := &types.RouterSpec{}
	//TODO: think througn van name, router name, secret names, etc.
	if options.SkupperName == "" {
		info, _ := cli.DockerInterface.Info()
		van.Name = info.Name
	} else {
		van.Name = options.SkupperName
	}

	if os.Getenv("QDROUTERD_MAGE") != "" {
		van.Transport.Image = os.Getenv("QDROUTERD_IMAGE")
	} else {
		van.Transport.Image = types.DefaultTransportImage
	}

	van.AuthMode = types.ConsoleAuthMode(options.AuthMode)
	van.Transport.LivenessPort = types.TransportLivenessPort
	van.Transport.Labels = map[string]string{
		"application":          types.TransportDeploymentName,
		"skupper.io/component": types.TransportComponentName,
		"prometheus.io/port":   "9090",
		"prometheus.io/scrape": "true",
	}

	routerConfig := qdr.InitialConfig(van.Name+"-${HOSTNAME}", siteId, options.IsEdge)
	routerConfig.AddAddress(qdr.Address{
		Prefix:       "mc",
		Distribution: "multicast",
	})
	routerConfig.AddListener(qdr.Listener{
		Host:        "0.0.0.0",
		Port:        9090,
		Role:        "normal",
		Http:        true,
		HttpRootDir: "disabled",
		Websockets:  false,
		Healthz:     true,
		Metrics:     true,
	})
	routerConfig.AddListener(qdr.Listener{
		Name: "amqp",
		Host: "localhost",
		Port: types.AmqpDefaultPort,
	})
	routerConfig.AddSslProfile(qdr.SslProfile{
		Name: "skupper-amqps",
	})
	routerConfig.AddListener(qdr.Listener{
		Name:             "amqps",
		Host:             "0.0.0.0",
		Port:             types.AmqpsDefaultPort,
		SslProfile:       "skupper-amqps",
		SaslMechanisms:   "EXTERNAL",
		AuthenticatePeer: true,
	})
	if options.EnableRouterConsole {
		if van.AuthMode == types.ConsoleAuthModeInternal {
			routerConfig.AddListener(qdr.Listener{
				Name:             types.ConsolePortName,
				Host:             "0.0.0.0",
				Port:             types.ConsoleDefaultServicePort,
				Http:             true,
				AuthenticatePeer: true,
			})
		} else if van.AuthMode == types.ConsoleAuthModeUnsecured {
			routerConfig.AddListener(qdr.Listener{
				Name: types.ConsolePortName,
				Host: "0.0.0.0",
				Port: types.ConsoleDefaultServicePort,
				Http: true,
			})
		}
	}
	if !options.IsEdge {
		routerConfig.AddSslProfile(qdr.SslProfile{
			Name: types.InterRouterProfile,
		})
		routerConfig.AddListener(qdr.Listener{
			Name:             "interior-listener",
			Host:             "0.0.0.0",
			Role:             qdr.RoleInterRouter,
			Port:             types.InterRouterListenerPort,
			SslProfile:       types.InterRouterProfile,
			SaslMechanisms:   "EXTERNAL",
			AuthenticatePeer: true,
		})
		routerConfig.AddListener(qdr.Listener{
			Name:             "edge-listener",
			Host:             "0.0.0.0",
			Role:             qdr.RoleEdge,
			Port:             types.EdgeListenerPort,
			SslProfile:       types.InterRouterProfile,
			SaslMechanisms:   "EXTERNAL",
			AuthenticatePeer: true,
		})
	}
	van.RouterConfig, _ = qdr.MarshalRouterConfig(routerConfig)

	envVars := []string{}
	if !options.IsEdge {
		envVars = append(envVars, "APPLICATION_NAME="+types.TransportDeploymentName)
		// TODO: auto_mesh for non k8s deploy
		//		envVars = append(envVars, "QDROUTERD_AUTO_MESH_DISCOVERY=QUERY")
	}
	if options.AuthMode == string(types.ConsoleAuthModeInternal) {
		envVars = append(envVars, "QDROUTERD_AUTO_CREATE_SASLDB_SOURCE=/etc/qpid-dispatch/sasl-users/")
		envVars = append(envVars, "QDROUTERD_AUTO_CREATE_SASLDB_PATH=/tmp/qdrouterd.sasldb")
	}
	if options.TraceLog {
		envVars = append(envVars, "PN_TRACE_FRM=1")
	}
	envVars = append(envVars, "QDROUTERD_CONF=/etc/qpid-dispatch/config/"+types.TransportConfigFile)
	envVars = append(envVars, "QDROUTERD_CONF_TYPE=json")
	envVars = append(envVars, "SKUPPER_SITE_ID="+siteId)
	van.Transport.EnvVar = envVars

	ports := nat.PortSet{}
	ports["5671/tcp"] = struct{}{}
	if options.AuthMode != "" {
		ports[nat.Port(strconv.Itoa(int(types.ConsoleDefaultServicePort))+"/tcp")] = struct{}{}
	}
	ports[nat.Port(strconv.Itoa(int(types.TransportLivenessPort)))+"/tcp"] = struct{}{}
	if !options.IsEdge {
		ports[nat.Port(strconv.Itoa(int(types.InterRouterListenerPort)))+"/tcp"] = struct{}{}
		ports[nat.Port(strconv.Itoa(int(types.EdgeListenerPort)))+"/tcp"] = struct{}{}
	}
	van.Transport.Ports = ports

	volumes := []string{
		"skupper",
		"skupper-amqps",
		"router-config",
	}
	if !options.IsEdge {
		volumes = append(volumes, "skupper-internal")
	}
	if options.AuthMode == string(types.ConsoleAuthModeInternal) {
		volumes = append(volumes, "skupper-console-users")
		volumes = append(volumes, "skupper-sasl-config")
	}
	van.Transport.Volumes = volumes

	// Note: use index to make directory, use index/value to make mount
	mounts := make(map[string]string)
	mounts[types.GetSkupperPath(types.CertsPath)] = "/etc/qpid-dispatch-certs"
	mounts[types.GetSkupperPath(types.ConnectionsPath)] = "/etc/qpid-dispatch/connections"
	mounts[types.GetSkupperPath(types.ConfigPath)] = "/etc/qpid-dispatch/config"
	mounts[types.GetSkupperPath(types.ConsoleUsersPath)] = "/etc/qpid-dispatch/sasl-users/"
	mounts[types.GetSkupperPath(types.SaslConfigPath)] = "/etc/sasl2"
	van.Transport.Mounts = mounts

	cas := []types.CertAuthority{}
	cas = append(cas, types.CertAuthority{Name: "skupper-ca"})
	if !options.IsEdge {
		cas = append(cas, types.CertAuthority{Name: "skupper-internal-ca"})
	}
	van.CertAuthoritys = cas

	credentials := []types.Credential{}
	credentials = append(credentials, types.Credential{
		CA:          "skupper-ca",
		Name:        "skupper-amqps",
		Subject:     "skupper-messaging",
		Hosts:       []string{"skupper-router"},
		ConnectJson: false,
		Post:        false,
	})
	credentials = append(credentials, types.Credential{
		CA:          "skupper-ca",
		Name:        "skupper",
		Subject:     "skupper-messaging",
		Hosts:       []string{},
		ConnectJson: true,
		Post:        false,
	})
	if !options.IsEdge {
		credentials = append(credentials, types.Credential{
			CA:          "skupper-internal-ca",
			Name:        "skupper-internal",
			Subject:     "skupper-internal",
			Hosts:       []string{"skupper-router"},
			ConnectJson: false,
			Post:        false,
		})
	}
	van.Credentials = credentials

	// Controller spec portion
	if os.Getenv("SKUPPER_CONTROLLER_IMAGE") != "" {
		van.Controller.Image = os.Getenv("SKUPPER_CONTROLLER_IMAGE")
	} else {
		van.Controller.Image = types.DefaultControllerImage
	}
	van.Controller.Labels = map[string]string{
		"application":          types.ControllerDeploymentName,
		"skupper.io/component": types.ControllerComponentName,
	}
	var skupperHost string
	if runtime.GOOS == "linux" {
		skupperHost = utils.GetInternalIP("docker0")
	} else {
		skupperHost = "host-gateway"
	}
	van.Controller.EnvVar = []string{
		"SKUPPER_SITE_ID=" + siteId,
		"SKUPPER_TMPDIR=" + os.Getenv("SKUPPER_TMPDIR"),
		"SKUPPER_PROXY_IMAGE=" + van.Controller.Image,
		"SKUPPER_HOST=" + skupperHost,
	}
	if options.MapToHost {
		van.Controller.EnvVar = append(van.Controller.EnvVar, "SKUPPER_MAP_TO_HOST=true")
	}
	if options.TraceLog {
		van.Controller.EnvVar = append(van.Controller.EnvVar, "PN_TRACE_FRM=1")
	}
	van.Controller.Mounts = map[string]string{
		types.GetSkupperPath(types.CertsPath) + "/" + "skupper": "/etc/messaging",
		types.GetSkupperPath(types.ServicesPath):                "/etc/messaging/services",
		"/var/run":                                              "/var/run",
	}

	return van, nil
}

// RouterCreate instantiates a VAN Router (transport and controller)
func (cli *VanClient) RouterCreate(options types.SiteConfigSpec) error {
	//TODO return error
	if options.EnableConsole {
		if options.AuthMode == string(types.ConsoleAuthModeInternal) || options.AuthMode == "" {
			options.AuthMode = string(types.ConsoleAuthModeInternal)
			if options.User == "" {
				options.User = "admin"
			}
			if options.Password == "" {
				options.Password = utils.RandomId(10)
			}
		} else {
			if options.User != "" {
				return fmt.Errorf("--router-console-user only valid when --router-console-auth=internal")
			}
			if options.Password != "" {
				return fmt.Errorf("--router-console-password only valid when --router-console-auth=internal")
			}
		}

	}

	// TODO check if resources already exist: either delete them all or error out
	// setup host dirs
	_ = os.RemoveAll(types.GetSkupperPath(types.HostPath))
	// create host dirs TODO this should not be here
	if err := os.MkdirAll(types.GetSkupperPath(types.HostPath), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(types.GetSkupperPath(types.SitesPath), 0755); err != nil {
		return err
	}

	sc, err := cli.SiteConfigCreate(options)
	if err != nil {
		return err
	}

	van, err := cli.GetRouterSpecFromOpts(options, sc.UID)
	if err != nil {
		return err
	}

	err = cli.DockerInterface.PullImage(van.Transport.Image, dockertypes.AuthConfig{}, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}

	err = cli.DockerInterface.PullImage(van.Controller.Image, dockertypes.AuthConfig{}, dockertypes.ImagePullOptions{})
	if err != nil {
		return err
	}

	for mnt := range van.Transport.Mounts {
		if err := os.Mkdir(mnt, 0755); err != nil {
			return err
		}
	}
	for _, v := range van.Transport.Volumes {
		if err := os.Mkdir(types.GetSkupperPath(types.CertsPath)+"/"+v, 0755); err != nil {
			return err
		}
	}

	// this one is needed by the controller
	if err := os.Mkdir(types.GetSkupperPath(types.ServicesPath), 0755); err != nil {
		return err
	}
	// create skupper-services file
	svcDefs := make(map[string]types.ServiceInterface)
	encoded, err := json.Marshal(svcDefs)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(types.GetSkupperPath(types.ServicesPath)+"/skupper-services", encoded, 0755)
	if err != nil {
		return err
	}

	// write qdrouterd configs

	err = ioutil.WriteFile(types.GetSkupperPath(types.ConfigPath)+"/qdrouterd.json", []byte(van.RouterConfig), 0755)
	if err != nil {
		return err
	}
	if options.EnableConsole && options.AuthMode == string(types.ConsoleAuthModeInternal) {
		config := `
pwcheck_method: auxprop
auxprop_plugin: sasldb
sasldb_path: /tmp/qdrouterd.sasldb
`
		err := ioutil.WriteFile(types.GetSkupperPath(types.SaslConfigPath)+"/qdrouterd.conf", []byte(config), 0755)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(types.GetSkupperPath(types.ConsoleUsersPath)+"/"+options.User, []byte(options.Password), 0755)
		if err != nil {
			return err
		}
	}

	// create user network
	_, err = docker.NewTransportNetwork(types.TransportNetworkName, cli.DockerInterface)
	if err != nil {
		return err
	}

	// fire up the containers
	transport, err := docker.NewTransportContainer(van, cli.DockerInterface)
	if err != nil {
		return err
	}

	for _, ca := range van.CertAuthoritys {
		ensureCA(ca.Name)
	}

	for _, cred := range van.Credentials {
		generateCredentials(cred.CA, cred.Name, cred.Subject, cred.Hosts, cred.ConnectJson)
	}

	//TODO : generate certs first?
	err = docker.StartContainer(transport.Name, cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Could not start transport container: %w", err)
	}

	controller, err := docker.NewControllerContainer(van, cli.DockerInterface)
	if err != nil {
		return err
	}

	err = docker.StartContainer(controller.Name, cli.DockerInterface)
	if err != nil {
		return fmt.Errorf("Could not start controller container: %w", err)
	}

	return nil
}
