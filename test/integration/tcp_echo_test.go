// +build integration

package integration

import (
	"fmt"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	"github.com/skupperproject/skupper-docker/pkg/docker/libdocker"
	"github.com/skupperproject/skupper/test/integration/tcp_echo"
)

var someTiemout time.Duration = 2 * time.Minute
var someTick time.Duration = 10 * time.Second

var someBackoff wait.Backoff = wait.Backoff{
	Steps:    int(someTiemout / someTick),
	Duration: someTick,
}

func TestTcpEcho(t *testing.T) {
	docker := libdocker.ConnectToDockerOrDie(0, 10*time.Second)

	isError := func(err error) bool {
		return err != nil
	}

	var current *dockertypes.ContainerJSON = nil
	var err error
	reterr := retry.OnError(someBackoff, isError, func() error {
		current, err = docker.InspectContainer("tcp-go-echo")
		if err != nil {
			fmt.Printf("waiting for container: error: %s", err.Error())
		}
		return err
	})

	if reterr != nil {
		fmt.Printf("Exposed container never showed up.")
		t.FailNow()
	}

	fmt.Printf("After service/container shows up, sleeping 20 seconds\n")
	time.Sleep(20 * time.Second)

	ip := current.NetworkSettings.Networks["skupper-network"].IPAddress
	fmt.Printf("serviceIP = %s\n", ip)

	for i := 0; i < 3; i++ {
		//sendReceive fails after 1 minute maximum
		err = tcp_echo.SendReceive(ip + ":9090")
		if err == nil {
			return
		}
		t.Logf("WARNING! try: %d failed!, retrying ...\n", i+1)
	}
	assert.Assert(t, err)
}
