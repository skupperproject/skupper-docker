package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
)

func (cli *VanClient) SiteConfigInspect(name string) (*types.SiteConfig, error) {
	sc := &types.SiteConfig{}

	scFile, err := ioutil.ReadFile(types.GetSkupperPath(types.SitesPath) + "/" + name + ".json")
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(scFile), &sc)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode json for site config definition: %w", err)
	}

	return sc, nil
}
