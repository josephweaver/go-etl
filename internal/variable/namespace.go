package variable

type Namespace string

const (
	NamespaceGlobalConfig          Namespace = "global_config"
	NamespaceClientEnvironment     Namespace = "client_env"
	NamespaceControllerEnvironment Namespace = "controller_env"
	NamespaceWorkerEnvironment     Namespace = "worker_env"
	NamespaceClientConfig          Namespace = "client_config"
	NamespaceControllerConfig      Namespace = "controller_config"
	NamespaceWorkerConfig          Namespace = "worker_config"
	NamespaceProjectConfig         Namespace = "project_config"
	NamespaceWorkflow              Namespace = "workflow"
	NamespaceOverride              Namespace = "override"
	NamespaceStep                  Namespace = "step"
	NamespaceFanOut                Namespace = "fanout"
	NamespaceAsset                 Namespace = "asset"
	NamespaceWorkItem              Namespace = "work_item"
	NamespaceRuntime               Namespace = "runtime"

	// Deprecated compatibility names. Existing workflow JSON and local demo
	// code still use these while the new config scopes are introduced.
	NamespaceGlobal  Namespace = "global"
	NamespaceBackend Namespace = "backend"
	NamespaceProject Namespace = "project"
)

var Precedence = []Namespace{
	NamespaceGlobalConfig,
	NamespaceClientEnvironment,
	NamespaceControllerEnvironment,
	NamespaceWorkerEnvironment,
	NamespaceClientConfig,
	NamespaceControllerConfig,
	NamespaceWorkerConfig,
	NamespaceProjectConfig,
	NamespaceWorkflow,
	NamespaceOverride,
	NamespaceStep,
	NamespaceFanOut,
	NamespaceAsset,
	NamespaceWorkItem,
	NamespaceRuntime,
}

func (n Namespace) Valid() bool {
	switch n {
	case NamespaceGlobalConfig,
		NamespaceClientEnvironment,
		NamespaceControllerEnvironment,
		NamespaceWorkerEnvironment,
		NamespaceClientConfig,
		NamespaceControllerConfig,
		NamespaceWorkerConfig,
		NamespaceProjectConfig,
		NamespaceWorkflow,
		NamespaceOverride,
		NamespaceStep,
		NamespaceFanOut,
		NamespaceAsset,
		NamespaceWorkItem,
		NamespaceRuntime,
		NamespaceGlobal,
		NamespaceBackend,
		NamespaceProject:
		return true
	default:
		return false
	}
}
