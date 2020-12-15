package client

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/skupperproject/skupper-docker/api/types"
	"gotest.tools/assert"
)

func TestConnectorCreateTokenInterior(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "connector")
	assert.Check(t, err, "Unable to create temporary directory")
	os.Setenv("SKUPPER_TMPDIR", tmpDir)
	defer os.RemoveAll(tmpDir)

	cli, err := NewClient()
	assert.Check(t, err, "Unable to create VAN client")

	scs := types.SiteConfigSpec{
		SkupperName:         "skupper",
		IsEdge:              false,
		EnableController:    true,
		EnableServiceSync:   true,
		EnableRouterConsole: false,
		EnableConsole:       true,
		AuthMode:            "unsecured",
		User:                "",
		Password:            "",
	}
	err = cli.RouterCreate(scs)
	time.Sleep(time.Second * 1)
	assert.Check(t, err, "Unable to create VAN router")

	err = cli.ConnectorTokenCreate("conn1", tmpDir+"/conn1.yaml")
	assert.Check(t, err, "Unable to create connector token")

	errors := cli.RouterRemove()
	assert.Assert(t, len(errors) == 0, "Error removing VAN router")
}

func TestConnectorCreateTokenEdge(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "connector")
	assert.Check(t, err, "Unable to create temporary directory")
	os.Setenv("SKUPPER_TMPDIR", tmpDir)
	defer os.RemoveAll(tmpDir)

	cli, err := NewClient()
	assert.Check(t, err, "Unable to create VAN client")

	scs := types.SiteConfigSpec{
		SkupperName:         "skupper",
		IsEdge:              true,
		EnableController:    true,
		EnableServiceSync:   true,
		EnableRouterConsole: false,
		EnableConsole:       true,
		AuthMode:            "unsecured",
		User:                "",
		Password:            "",
	}
	err = cli.RouterCreate(scs)
	time.Sleep(time.Second * 1)
	assert.Check(t, err, "Unable to create VAN router")

	err = cli.ConnectorTokenCreate("conn1", tmpDir+"/conn1.yaml")
	assert.Equal(t, err.Error(), "Edge mode transport configuration cannot accept connections")

	errors := cli.RouterRemove()
	assert.Assert(t, len(errors) == 0, "Error removing VAN router")
}

func TestConnectorCreateNoFileError(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "connector")
	assert.Check(t, err, "Unable to create temporary directory")
	os.Setenv("SKUPPER_TMPDIR", tmpDir)
	defer os.RemoveAll(tmpDir)

	cli, err := NewClient()
	assert.Check(t, err, "Unable to create VAN client")

	scs := types.SiteConfigSpec{
		SkupperName:         "skupper",
		IsEdge:              false,
		EnableController:    true,
		EnableServiceSync:   true,
		EnableRouterConsole: false,
		EnableConsole:       true,
		AuthMode:            "unsecured",
		User:                "",
		Password:            "",
	}
	err = cli.RouterCreate(scs)
	time.Sleep(time.Second * 1)
	assert.Check(t, err, "Unable to create VAN router")

	_, err = cli.ConnectorCreate(tmpDir+"/conn1.yaml", types.ConnectorCreateOptions{})
	assert.Equal(t, err.Error(), "Failed to make connector: Could not read connection token: open "+tmpDir+"/conn1.yaml: no such file or directory")
	errors := cli.RouterRemove()
	assert.Assert(t, len(errors) == 0, "Error removing VAN router")
}

func TestConnectorCreateFakeOut(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "connector")
	assert.Check(t, err, "Unable to create temporary directory")
	os.Setenv("SKUPPER_TMPDIR", tmpDir)
	defer os.RemoveAll(tmpDir)

	cli, err := NewClient()
	assert.Check(t, err, "Unable to create VAN client")

	scs := types.SiteConfigSpec{
		SkupperName:         "skupper",
		IsEdge:              false,
		EnableController:    true,
		EnableServiceSync:   true,
		EnableRouterConsole: false,
		EnableConsole:       true,
		AuthMode:            "unsecured",
		User:                "",
		Password:            "",
	}
	err = cli.RouterCreate(scs)
	time.Sleep(time.Second * 1)
	assert.Check(t, err, "Unable to create VAN router")

	err = cli.ConnectorTokenCreate("subject1", tmpDir+"/conn1.yaml")
	assert.Check(t, err, "Unable to create token")

	err = cli.ConnectorTokenCreate("subjec2", tmpDir+"/conn2.yaml")
	assert.Check(t, err, "Unable to create token")

	errors := cli.RouterRemove()
	assert.Assert(t, len(errors) == 0, "Error removing VAN router")
	time.Sleep(time.Second * 1)

	err = cli.RouterCreate(scs)
	assert.Check(t, err, "Unable to create VAN router")
	time.Sleep(time.Second * 1)

	_, err = cli.ConnectorCreate(tmpDir+"/conn1.yaml", types.ConnectorCreateOptions{})
	assert.Check(t, err, "Unable to create connector")

	_, err = cli.ConnectorCreate(tmpDir+"/conn2.yaml", types.ConnectorCreateOptions{})
	assert.Check(t, err, "Unable to create connector")

	conns, err := cli.ConnectorList()
	assert.Check(t, err, "Unable to list connectors")
	assert.Assert(t, len(conns) == 2, "Error with the number of connectors")

	_, err = cli.ConnectorInspect("conn1")
	assert.Check(t, err, "Unable to inspect connector")

	err = cli.ConnectorRemove("conn1")
	assert.Check(t, err, "Unable to remove connector")

	errors = cli.RouterRemove()
	assert.Assert(t, len(errors) == 0, "Error removing VAN router")
}
