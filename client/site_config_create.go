package client

import (
	"encoding/json"
	"io/ioutil"

	"github.com/google/uuid"

	"github.com/skupperproject/skupper-docker/api/types"
)

func NewUUID() string {
	return uuid.New().String()
}

func (cli *VanClient) SiteConfigCreate(spec types.SiteConfigSpec) (*types.SiteConfig, error) {
	sc := &types.SiteConfig{
		Spec: spec,
		UID:  NewUUID(),
	}
	encoded, err := json.Marshal(sc)
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(types.GetSkupperPath(types.SitesPath)+"/"+types.DefaultBridgeName+".json", encoded, 0755)
	if err != nil {
		return nil, err
	}
	return sc, nil
}
