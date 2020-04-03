package client

import (
	"log"

	"github.com/skupperproject/skupper-cli/pkg/certs"
	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
)

func (cli *VanClient) VanConnectorTokenCreate(subject string, secretFile string) error {
	// verify that the transport is interior mode
	current, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		// TODO: is not found versus error
		log.Println("Unable to retrieve transport container (need init?)", err.Error())
		return err
	}

	if !qdr.IsInterior(current) {
		log.Println("Edge mode transport configuration cannot accept connections")
		return nil
	}

	caData, err := getCertData("skupper-internal-ca")
	if err != nil {
		log.Println("Unable to retrieve CA data", err.Error())
		return nil
	}

	ipAddr := string(current.NetworkSettings.Networks["skupper-network"].IPAddress)
	annotations := make(map[string]string)
	annotations["inter-router-port"] = "55671"
	annotations["inter-router-host"] = ipAddr

	certData := certs.GenerateCertificateData(subject, subject, ipAddr, caData)
	certs.PutCertificateData(subject, secretFile, certData, annotations)

	return nil
}
