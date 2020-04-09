/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package qdr

import (
	"bytes"

	dockertypes "github.com/docker/docker/api/types"
	dockerstdcopy "github.com/docker/docker/pkg/stdcopy"
	"github.com/skupperproject/skupper-docker/pkg/docker/libdocker"
)

// TODO: rethink where this should be placed in pkg dir

type ExecResult struct {
	ExitCode  int
	outBuffer *bytes.Buffer
	errBuffer *bytes.Buffer
}

func (res *ExecResult) Stderr() string {
	return res.errBuffer.String()
}

func (res *ExecResult) Stdout() string {
	return res.outBuffer.String()
}

func Exec(dd libdocker.Interface, id string, cmd []string) (ExecResult, error) {
	execConfig := dockertypes.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	createResponse, err := dd.CreateExec(id, execConfig)
	if err != nil {
		return ExecResult{}, err
	}
	execID := createResponse.ID

	// run with stdout and stderr attached
	attachResponse, err := dd.AttachExec(execID, dockertypes.ExecStartCheck{})
	if err != nil {
		return ExecResult{}, err
	}
	defer attachResponse.Close()

	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error, 1)

	go func() {
		_, err = dockerstdcopy.StdCopy(&outBuf, &errBuf, attachResponse.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return ExecResult{}, err
		}
		break
	}

	inspectResponse, err := dd.InspectExec(execID)
	if err != nil {
		return ExecResult{}, err
	}

	return ExecResult{ExitCode: inspectResponse.ExitCode, outBuffer: &outBuf, errBuffer: &errBuf}, nil
}
