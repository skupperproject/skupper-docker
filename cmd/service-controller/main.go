package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/client"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

func describe(i interface{}) {
	fmt.Printf("(%v, %T)\n", i, i)
	fmt.Println()
}

var onlyOneSignalHandler = make(chan struct{})
var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

func SetupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
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

func main() {
	siteId := os.Getenv("SKUPPER_SITE_ID")

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := SetupSignalHandler()

	cli, err := client.NewClient()
	if err != nil {
		log.Fatal("Error getting van client", err.Error())
	}

	tlsConfig, err := getTlsConfig(true, types.ControllerConfigPath+"tls.crt", types.ControllerConfigPath+"tls.key", types.ControllerConfigPath+"ca.crt")
	if err != nil {
		log.Fatal("Error getting van client: ", err.Error())
	}

	controller, err := NewController(cli, siteId, tlsConfig)
	if err != nil {
		log.Fatal("Error getting new controller: ", err.Error())
	}

	log.Println("Waiting for the Skupper router component to start")
	_, err = docker.WaitForContainerStatus("skupper-router", "running", time.Second*180, time.Second*5, cli.DockerInterface)
	if err != nil {
		log.Fatal("Failed waiting for router to be running", err.Error())
	}

	// start the controller workers
	if err = controller.Run(stopCh); err != nil {
		log.Fatal("Error running controller: ", err.Error())
	}

}
