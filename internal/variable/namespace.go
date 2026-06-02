package variable

type Namespace string

const (
	NamespaceClientEnvironment     Namespace = "client_env"
	NamespaceControllerEnvironment Namespace = "controller_env"
	NamespaceWorkerEnvironment     Namespace = "worker_env"
	NamespaceGlobal                Namespace = "global"
	NamespaceBackend               Namespace = "backend"
	NamespaceProject               Namespace = "project"
	NamespaceWorkflow              Namespace = "workflow"
	NamespaceOverride              Namespace = "override"
)

var Precedence = []Namespace{
	NamespaceClientEnvironment,
	NamespaceControllerEnvironment,
	NamespaceWorkerEnvironment,
	NamespaceGlobal,
	NamespaceBackend,
	NamespaceProject,
	NamespaceWorkflow,
	NamespaceOverride,
}

func (n Namespace) Valid() bool {
	switch n {
	case NamespaceClientEnvironment,
		NamespaceControllerEnvironment,
		NamespaceWorkerEnvironment,
		NamespaceGlobal,
		NamespaceBackend,
		NamespaceProject,
		NamespaceWorkflow,
		NamespaceOverride:
		return true
	default:
		return false
	}
}
