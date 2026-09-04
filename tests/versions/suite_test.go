package versions_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/hooks"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
)

var sharedClients *clients.Clients

var oc = occmd.OC{}

// lastNamespace tracks the current test namespace for diagnostic collection.
// Set in BeforeEach by test specs; read in ReportAfterEach by diagnostics collector.
var lastNamespace string

func TestVersions(t *testing.T) {
	config.MustLoadEnvironment()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Versions Suite", Label("versions"))
}

// clientConfig holds the serialized configuration passed between parallel nodes.
type clientConfig struct {
	Kubeconfig      string `json:"kubeconfig"`
	Cluster         string `json:"cluster"`
	Context         string `json:"context"`
	TargetNamespace string `json:"targetNamespace"`
}

var _ = SynchronizedBeforeSuite(
	// Node 1 only: validate cluster connectivity and serialize config
	func() []byte {
		// Verify cluster is reachable by creating clients
		cs, err := clients.NewClientsWithContext(
			config.Flags.Kubeconfig,
			config.Flags.Cluster,
			config.Flags.Context,
			config.TargetNamespace,
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Kubernetes clients on node 1")
		_ = cs // validation only on node 1

		cfg := clientConfig{
			Kubeconfig:      config.Flags.Kubeconfig,
			Cluster:         config.Flags.Cluster,
			Context:         config.Flags.Context,
			TargetNamespace: config.TargetNamespace,
		}
		data, err := json.Marshal(cfg)
		Expect(err).NotTo(HaveOccurred(), "Failed to serialize client config")
		return data
	},
	// All nodes: deserialize config and create node-local clients
	func(data []byte) {
		var cfg clientConfig
		Expect(json.Unmarshal(data, &cfg)).To(Succeed(), "Failed to deserialize client config")

		var err error
		sharedClients, err = clients.NewClientsWithContext(cfg.Kubeconfig, cfg.Cluster, cfg.Context, cfg.TargetNamespace)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Kubernetes clients")
	},
)

var _ = AfterSuite(func() {
	hooks.CleanupNamespaces()
	_ = config.RemoveTempDir()
})

// Automatically create namespace per Describe block
var _ = hooks.AutoNamespacePerDescribe(&lastNamespace, func() *clients.Clients { return sharedClients })

// Collect diagnostics (pod logs, events, resource state) on test failure.
// Disabled for cleaner console output
// var _ = ReportAfterEach(diagnostics.CollectOnFailure(&lastNamespace))
