package client

import (
	"fmt"

	"github.com/skupperproject/skupper-docker/pkg/docker"
	"github.com/skupperproject/skupper-docker/pkg/qdr"
	"github.com/skupperproject/skupper/pkg/certs"
)

func (cli *VanClient) VanConnectorTokenCreate(subject string, secretFile string) error {
	// verify that the transport is interior mode
	current, err := docker.InspectContainer("skupper-router", cli.DockerInterface)
	if err != nil {
		// TODO: is not found versus error
		return fmt.Errorf("Unable to retrieve transport container (need init?): %w", err)
	}

	if !qdr.IsInterior(current) {
		return fmt.Errorf("Edge mode transport configuration cannot accept connections")
	}

	caData, err := getCertData("skupper-internal-ca")
	if err != nil {
		return fmt.Errorf("Unable to retrieve CA data: %w", err)
	}

	ipAddr := string(current.NetworkSettings.Networks["skupper-network"].IPAddress)
	annotations := make(map[string]string)
	annotations["inter-router-port"] = "55671"
	annotations["inter-router-host"] = ipAddr

	// TODO err return from certs pkg
	certData := certs.GenerateCertificateData(subject, subject, ipAddr, caData)
	certs.PutCertificateData(subject, secretFile, certData, annotations)

	return nil
}
