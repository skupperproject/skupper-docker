package types

type ConnectorCreateOptions struct {
	Name string
	Cost int32
}

type RouterCreateOptions struct {
	SkupperName         string
	IsEdge              bool
	EnableController    bool
	EnableServiceSync   bool
	EnableConsole       bool
	EnableRouterConsole bool
	AuthMode            string
	User                string
	Password            string
	ClusterLocal        bool
	Replicas            int32
}

type ServiceInterfaceCreateOptions struct {
	Protocol   string
	Address    string
	Port       int
	TargetPort int
	Headless   bool
}

type ServiceInterfaceRemoveOptions struct {
	Address string
}

type RouterInspectResponse struct {
	Status            RouterStatusSpec
	TransportVersion  string
	ControllerVersion string
	ExposedServices   int
}

type ConnectorInspectResponse struct {
	Connector *Connector
	Connected bool
}

type RouterStatusSpec struct {
	Mode           string                  `json:"mode,omitempty"`
	State          string                  `json:"state,omitempty"`
	ConnectedSites TransportConnectedSites `json:"connectedSites,omitempty"`
	BindingsCount  int                     `json:"bindingsCount,omitempty"`
}

type VanClientInterface interface {
	ConnectorCreate(secretFile string, options ConnectorCreateOptions) (string, error)
	ConnectorInspect(name string) (*ConnectorInspectResponse, error)
	ConnectorList() ([]*Connector, error)
	ConnectorRemove(name string) error
	ConnectorTokenCreate(subject string, secretFile string) error
	RouterCreate(options RouterCreateOptions) error
	RouterInspect() (*RouterInspectResponse, error)
	RouterRemove() []error
	ServiceInterfaceCreate(targetType string, targetName string, options ServiceInterfaceCreateOptions) error
	ServiceInterfaceInspect(address string) (*ServiceInterface, error)
	ServiceInterfaceList() ([]ServiceInterface, error)
	ServiceInterfaceRemove(targetType string, targetName string, options ServiceInterfaceRemoveOptions) error
}
