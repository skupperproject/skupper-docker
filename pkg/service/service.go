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

package service

type ExposeOptions struct {
	Protocol   string
	Address    string
	Port       int
	TargetPort int
	Headless   bool
}

type Service struct {
	Address  string          `json:"address"`
	Protocol string          `json:"protocol"`
	Port     int             `json:"port"`
	Headless *Headless       `json:"headles,omitempty"`
	Targets  []ServiceTarget `json:"targets,omitempty"`
}

type ServiceTarget struct {
	Name       string `json:"name"`
	Selector   string `json:"selector"`
	TargetPort int    `json:"targetPort,omitempty"`
}

type Headless struct {
	Name       string `json:"name"`
	Size       int    `json:"size"`
	TargetPort int    `json:"targetPort,omitempty"`
}
