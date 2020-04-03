package client

import (
	"io/ioutil"
	"log"
	"os"
	"strconv"

	dockertypes "github.com/docker/docker/api/types"

	"github.com/docker/go-connections/nat"

	"github.com/skupperproject/skupper-cli/pkg/certs"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/utils"
	"github.com/skupperproject/skupper-docker/pkg/utils/configs"
)

// TODO: move all the certs stuff to a package?
func getCertData(name string) (certs.CertificateData, error) {
	certData := certs.CertificateData{}
	certPath := types.CertPath + name

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

func generateCredentials(ca string, name string, subject string, hosts string, includeConnectJson bool) {
	caData, _ := getCertData(ca)
	certData := certs.GenerateCertificateData(name, subject, hosts, caData)

	for k, v := range certData {
		if err := ioutil.WriteFile(types.CertPath+name+"/"+k, v, 0755); err != nil {
			log.Fatal("Failed to write certificate file: ", err.Error())
		}
	}

	if includeConnectJson {
		certData["connect.json"] = []byte(configs.ConnectJson())
		if err := ioutil.WriteFile(types.CertPath+name+"/connect.json", []byte(configs.ConnectJson()), 0755); err != nil {
			log.Fatal("Failed to write connect file: ", err.Error())
		}
	}

}

func ensureCA(name string) certs.CertificateData {

	// check if existing by looking at path/dir, if not create dir to persist
	caData := certs.GenerateCACertificateData(name, name)

	if err := os.Mkdir(types.CertPath+name, 0755); err != nil {
		log.Fatal("Failed to create certificate directory: ", err.Error())
	}

	for k, v := range caData {
		if err := ioutil.WriteFile(types.CertPath+name+"/"+k, v, 0755); err != nil {
			log.Fatal("Failed to write CA certificate file: ", err.Error())
		}
	}

	return caData
}

func GetVanRouterSpecFromOpts(options types.VanRouterCreateOptions, client *VanClient) (*types.VanRouterSpec, error) {
	van := &types.VanRouterSpec{}
	//TODO: think througn van name, router name, secret names, etc.
	if options.SkupperName == "" {
		info, _ := client.DockerInterface.Info()
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

	listeners := []types.Listener{}
	interRouterListeners := []types.Listener{}
	edgeListeners := []types.Listener{}
	sslProfiles := []types.SslProfile{}
	listeners = append(listeners, types.Listener{
		Name: "amqp",
		Host: "localhost",
		Port: 5672,
	})
	sslProfiles = append(sslProfiles, types.SslProfile{
		Name: "skupper-amqps",
	})
	//TODO: vcabbage issue with EXTERNAL, requires ANONYMOUS,false
	listeners = append(listeners, types.Listener{
		Name:             "amqps",
		Host:             "0.0.0.0",
		Port:             5671,
		SslProfile:       "skupper-amqps",
		SaslMechanisms:   "ANONYMOUS",
		AuthenticatePeer: false,
	})
	if van.AuthMode == types.ConsoleAuthModeInternal {
		listeners = append(listeners, types.Listener{
			Name:             types.ConsolePortName,
			Host:             "0.0.0.0",
			Port:             types.ConsoleDefaultServicePort,
			Http:             true,
			AuthenticatePeer: true,
		})
	} else if van.AuthMode == types.ConsoleAuthModeUnsecured {
		listeners = append(listeners, types.Listener{
			Name: types.ConsolePortName,
			Host: "0.0.0.0",
			Port: types.ConsoleDefaultServicePort,
			Http: true,
		})
	}
	if !options.IsEdge {
		sslProfiles = append(sslProfiles, types.SslProfile{
			Name: "skupper-internal",
		})
		interRouterListeners = append(interRouterListeners, types.Listener{
			Name:             "interior-listener",
			Host:             "0.0.0.0",
			Port:             types.InterRouterListenerPort,
			SslProfile:       types.InterRouterProfile,
			SaslMechanisms:   "EXTERNAL",
			AuthenticatePeer: true,
		})
		edgeListeners = append(edgeListeners, types.Listener{
			Name:             "edge-listener",
			Host:             "0.0.0.0",
			Port:             types.EdgeListenerPort,
			SslProfile:       types.InterRouterProfile,
			SaslMechanisms:   "EXTERNAL",
			AuthenticatePeer: true,
		})
	}

	// TODO: remove redundancy, needed for now for config template
	van.Assembly.Name = van.Name
	if options.IsEdge {
		van.Assembly.Mode = string(types.TransportModeEdge)
	} else {
		van.Assembly.Mode = string(types.TransportModeInterior)
	}
	van.Assembly.Listeners = listeners
	van.Assembly.InterRouterListeners = interRouterListeners
	van.Assembly.EdgeListeners = edgeListeners
	van.Assembly.SslProfiles = sslProfiles

	envVars := []string{}
	if !options.IsEdge {
		envVars = append(envVars, "APPLICATION_NAME="+types.TransportDeploymentName)
		envVars = append(envVars, "QDROUTERD_AUTO_MESH_DISCOVERY=QUERY")
	}
	if options.AuthMode == string(types.ConsoleAuthModeInternal) {
		envVars = append(envVars, "QDROUTERD_AUTO_CREATE_SASLDB_SOURCE=/etc/qpid-dispatch/sasl-users/")
		envVars = append(envVars, "QDROUTERD_AUTO_CREATE_SASLDB_PATH=/tmp/qdrouterd.sasldb")
	}
	// envVars = append(envVars, "PN_TRACE_FRM=1")
	envVars = append(envVars, "QDROUTERD_CONF="+configs.QdrouterdConfig(&van.Assembly))
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
	mounts[types.CertPath] = "/etc/qpid-dispatch-certs"
	mounts[types.ConnPath] = "/etc/qpid-dispatch/connections"
	mounts[types.ConsoleUsersPath] = "/etc/qpid-dispatch/sasl-users/"
	mounts[types.SaslConfigPath] = "/etc/sasl2"
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
		Subject:     "skupper-router",
		Hosts:       "skupper-router",
		ConnectJson: false,
		Post:        false,
	})
	credentials = append(credentials, types.Credential{
		CA:          "skupper-ca",
		Name:        "skupper",
		Subject:     "skupper-router",
		Hosts:       "",
		ConnectJson: true,
		Post:        false,
	})
	if !options.IsEdge {
		credentials = append(credentials, types.Credential{
			CA:          "skupper-internal-ca",
			Name:        "skupper-internal",
			Subject:     "skupper-internal",
			Hosts:       "skupper-internal",
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
	van.Controller.EnvVar = []string{
		"SKUPPER_PROXY_IMAGE=" + van.Controller.Image,
		"SKUPPER_SERVICE_SYNC_ORIGIN=" + utils.RandomId(10),
	}
	van.Controller.Mounts = map[string]string{
		types.CertPath + "skupper": "/etc/messaging",
		types.ServicePath:          "/etc/messaging/services",
		"/var/run":                 "/var/run",
	}

	return van, nil
}

// VanRouterCreate instantiates a VAN Router (transport and controller)
func (cli *VanClient) VanRouterCreate(options types.VanRouterCreateOptions) error {
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
				log.Println("--router-console-user only valid when --router-console-auth=internal")
			}
			if options.Password != "" {
				log.Println("--router-console-password only valid when --router-console-auth=internal")
			}
		}
	}

	van, err := GetVanRouterSpecFromOpts(options, cli)
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

	// setup host dirs
	_ = os.RemoveAll(types.HostPath)
	// create host dirs TODO this should not be here
	if err := os.MkdirAll(types.HostPath, 0755); err != nil {
		return err
	}
	for mnt, _ := range van.Transport.Mounts {
		if err := os.Mkdir(mnt, 0755); err != nil {
			return err
		}
	}
	for _, v := range van.Transport.Volumes {
		if err := os.Mkdir(types.CertPath+v, 0755); err != nil {
			return err
		}
	}
	// this one is needed by the controller
	if err := os.Mkdir(types.ServicePath, 0755); err != nil {
		return err
	}

	// create user network
	_, err = docker.NewTransportNetwork("skupper-network", cli.DockerInterface)
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
		log.Println("Could not start transport container", err)
	}

	controller, err := docker.NewControllerContainer(van, cli.DockerInterface)
	if err != nil {
		return err
	}

	err = docker.StartContainer(controller.Name, cli.DockerInterface)
	if err != nil {
		log.Println("Could not start controller container", err)
	}

	return nil
}
