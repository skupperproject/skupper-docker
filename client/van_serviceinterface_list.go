package client

import (
	"encoding/json"
	"io/ioutil"

	"github.com/skupperproject/skupper-docker/api/types"
)

func (cli *VanClient) VanServiceInterfaceList() ([]*types.ServiceInterface, error) {
	var vsis []*types.ServiceInterface

	files, err := ioutil.ReadDir(types.ServicePath)
	if err != nil {
		return vsis, err
	}

	for _, f := range files {
		encoded, _ := ioutil.ReadFile(types.ServicePath + f.Name())
		si := types.ServiceInterface{}
		err = json.Unmarshal([]byte(encoded), &si)
		if err != nil {
			return vsis, err
		} else {
			vsis = append(vsis, &si)
		}
	}

	return vsis, err
}
