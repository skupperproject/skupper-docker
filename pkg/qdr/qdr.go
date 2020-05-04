package qdr

import (
	"regexp"

	dockertypes "github.com/docker/docker/api/types"

	"github.com/skupperproject/skupper-docker/api/types"
	"github.com/skupperproject/skupper-docker/pkg/docker"
)

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
