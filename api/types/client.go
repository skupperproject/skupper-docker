package types

type ConnectorCreateOptions struct {
	Name string
	Cost int32
}

type SiteConfig struct {
	Spec SiteConfigSpec
	UID  string
}

type SiteConfigSpec struct {
	SkupperName         string
	SkupperNamespace    string
	IsEdge              bool
	EnableController    bool
	EnableServiceSync   bool
	EnableRouterConsole bool
	EnableConsole       bool
	AuthMode            string
	User                string
	Password            string
	MapToHost           bool
	Replicas            int32
	TraceLog            bool
}

type ServiceInterfaceCreateOptions struct {
	Protocol   string
	Address    string
	Port       int
	TargetPort int
	Headless   bool
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
	RouterCreate(options SiteConfigSpec) error
	RouterInspect() (*RouterInspectResponse, error)
	RouterRemove() []error
	ServiceInterfaceBind(service *ServiceInterface, targetType string, targetName string, protocol string, targetPort int) error
	ServiceInterfaceCreate(service *ServiceInterface) error
	ServiceInterfaceInspect(address string) (*ServiceInterface, error)
	ServiceInterfaceList() ([]ServiceInterface, error)
	ServiceInterfaceRemove(address string) error
	ServiceInterfaceUnbind(targetType string, targetName string, address string, deleteIfNoTargets bool) error
	SiteConfigInspect(name string) (*SiteConfig, error)
}
