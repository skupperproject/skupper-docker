package client

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/skupperproject/skupper-docker/api/types"
	"gotest.tools/assert"
)

func TestRouterCreateDefaults(t *testing.T) {
	testCases := []struct {
		doc                 string
		expectedError       string
		tmpDir              string
		skupperName         string
		isEdge              bool
		enableController    bool
		enableServiceSync   bool
		enableRouterConsole bool
		enableConsole       bool
		authMode            string
		user                string
		password            string
		containersExpected  []string
		networksExpected    []string
		servicesExpected    []string
	}{
		{
			doc:                 "test one",
			expectedError:       "",
			skupperName:         "skupper1",
			tmpDir:              "",
			isEdge:              false,
			enableController:    true,
			enableRouterConsole: false,
			enableConsole:       false,
			authMode:            "",
			user:                "",
			password:            "",
			containersExpected:  []string{"skupper-router", "skupper-service-controller"},
			networksExpected:    []string{"skupper-network"},
			servicesExpected:    []string{},
		},
		{
			doc:                 "test two",
			expectedError:       "",
			skupperName:         "",
			tmpDir:              "",
			isEdge:              false,
			enableController:    true,
			enableRouterConsole: true,
			enableConsole:       true,
			authMode:            "internal",
			user:                "fred",
			password:            "flinstone",
			containersExpected:  []string{"skupper-router", "skupper-service-controller"},
			networksExpected:    []string{"skupper-network"},
			servicesExpected:    []string{},
		},
		{
			doc:                 "test three",
			expectedError:       "",
			skupperName:         "skupper3",
			tmpDir:              "",
			isEdge:              false,
			enableController:    true,
			enableRouterConsole: true,
			enableConsole:       true,
			authMode:            "unsecured",
			user:                "",
			password:            "",
			containersExpected:  []string{"skupper-router", "skupper-service-controller"},
			networksExpected:    []string{"skupper-network"},
			servicesExpected:    []string{},
		},
		{
			doc:                 "test four",
			expectedError:       "",
			skupperName:         "skupper4",
			tmpDir:              "",
			isEdge:              false,
			enableController:    true,
			enableRouterConsole: true,
			enableConsole:       true,
			authMode:            "internal",
			user:                "",
			password:            "",
			containersExpected:  []string{"skupper-router", "skupper-service-controller"},
			networksExpected:    []string{"skupper-network"},
			servicesExpected:    []string{},
		},
		{
			doc:                 "test five",
			expectedError:       "--router-console-user only valid when --router-console-auth=internal",
			skupperName:         "skupper5",
			tmpDir:              "",
			isEdge:              false,
			enableController:    true,
			enableRouterConsole: true,
			enableConsole:       true,
			authMode:            "unsecured",
			user:                "fred",
			password:            "flintsone",
			containersExpected:  []string{"skupper-router", "skupper-service-controller"},
			networksExpected:    []string{"skupper-network"},
			servicesExpected:    []string{},
		},
		{
			doc:                 "test six",
			expectedError:       "--router-console-password only valid when --router-console-auth=internal",
			skupperName:         "skupper6",
			tmpDir:              "",
			isEdge:              false,
			enableController:    true,
			enableRouterConsole: true,
			enableConsole:       true,
			authMode:            "unsecured",
			user:                "",
			password:            "flintsone",
			containersExpected:  []string{"skupper-router", "skupper-service-controller"},
			networksExpected:    []string{"skupper-network"},
			servicesExpected:    []string{},
		},
	}

	for _, c := range testCases {
		tmpDir, err := ioutil.TempDir("", "router")
		assert.Check(t, err, c.doc)
		os.Setenv("SKUPPER_TMPDIR", tmpDir)
		defer os.RemoveAll(tmpDir)

		cli, err := NewClient()
		assert.Check(t, err, c.doc)

		scs := types.SiteConfigSpec{
			SkupperName:         c.skupperName,
			IsEdge:              c.isEdge,
			EnableController:    c.enableController,
			EnableServiceSync:   true,
			EnableRouterConsole: c.enableRouterConsole,
			EnableConsole:       c.enableConsole,
			AuthMode:            c.authMode,
			User:                c.user,
			Password:            c.password,
		}

		err = cli.RouterCreate(scs)
		time.Sleep(time.Second * 1)
		if c.expectedError == "" {
			assert.Check(t, err, c.doc)
		} else {
			assert.Equal(t, err.Error(), c.expectedError)
		}

		if c.expectedError == "" {
			_, err := cli.SiteConfigInspect(types.DefaultBridgeName)
			assert.Check(t, err, c.doc)

			vir, err := cli.RouterInspect()
			assert.Check(t, err, c.doc)
			assert.Assert(t, vir.Status.State == "running", c.doc)
			assert.Assert(t, vir.Status.Mode == string(types.TransportModeInterior), c.doc)
		}

		errors := cli.RouterRemove()
		assert.Assert(t, len(errors) == 0, c.doc)

	}
}
