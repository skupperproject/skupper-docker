package docker

import (
	"strings"
)

func FindEnvVar(env []string, name string) string {
	for _, v := range env {
		if strings.HasPrefix(v, name) {
			return strings.TrimPrefix(v, name+"=")
		}
	}
	return ""
}

func SetEnvVar(current []string, name string, value string) []string {
	updated := []string{}
	for _, v := range current {
		if strings.HasPrefix(v, name) {
			updated = append(updated, name+"="+value)
		} else {
			updated = append(updated, v)
		}
	}
	return updated
}
