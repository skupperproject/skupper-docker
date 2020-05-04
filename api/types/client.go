package types

type VanConnectorCreateOptions struct {
	Name string
	Cost int32
}

type VanRouterCreateOptions struct {
	SkupperName       string
	IsEdge            bool
	EnableController  bool
	EnableServiceSync bool
	EnableConsole     bool
	AuthMode          string
	User              string
	Password          string
	ClusterLocal      bool
	Replicas          int32
}

type VanServiceInterfaceCreateOptions struct {
	Protocol   string
	Address    string
	Port       int
	TargetPort int
	Headless   bool
}

type VanServiceInterfaceRemoveOptions struct {
	Address string
}

type VanRouterInspectResponse struct {
	Status            VanRouterStatusSpec
	TransportVersion  string
	ControllerVersion string
	ExposedServices   int
}

type VanConnectorInspectResponse struct {
	Connector *Connector
	Connected bool
}

type VanRouterStatusSpec struct {
	Mode                   string                  `json:"mode,omitempty"`
	TransportReadyReplicas int32                   `json:"transportReadyReplicas,omitempty"`
	ConnectedSites         TransportConnectedSites `json:"connectedSites,omitempty"`
	BindingsCount          int                     `json:"bindingsCount,omitempty"`
}
