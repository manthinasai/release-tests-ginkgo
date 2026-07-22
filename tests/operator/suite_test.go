package operator_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega
	"github.com/tektoncd/operator/test/utils"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/hooks"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var sharedClients *clients.Clients

// lastNamespace tracks the current test namespace for diagnostic collection.
// Set in BeforeEach by test specs; read in ReportAfterEach by diagnostics collector.
var lastNamespace string

// oc is the package-level OpenShift CLI helper used by all operator test files.
var oc = occmd.OC{}

func TestOperator(t *testing.T) {
	config.MustLoadEnvironment()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator Suite", Label("operator"))
}

// clientConfig holds the serialized configuration passed between parallel nodes.
type clientConfig struct {
	Kubeconfig      string `json:"kubeconfig"`
	Cluster         string `json:"cluster"`
	TargetNamespace string `json:"targetNamespace"`
}

var _ = SynchronizedBeforeSuite(
	// Node 1 only: validate cluster connectivity and serialize config
	func() []byte {
		// Verify cluster is reachable by creating clients
		cs, err := clients.NewClients(
			config.Flags.Kubeconfig,
			config.Flags.Cluster,
			config.TargetNamespace,
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Kubernetes clients on node 1")
		_ = cs // validation only on node 1

		cfg := clientConfig{
			Kubeconfig:      config.Flags.Kubeconfig,
			Cluster:         config.Flags.Cluster,
			TargetNamespace: config.TargetNamespace,
		}
		data, err := json.Marshal(cfg)
		Expect(err).NotTo(HaveOccurred(), "Failed to serialize client config")
		return data
	},
	// All nodes: deserialize config, create node-local clients, and seed the store.
	func(data []byte) {
		var cfg clientConfig
		Expect(json.Unmarshal(data, &cfg)).To(Succeed(), "Failed to deserialize client config")

		var err error
		sharedClients, err = clients.NewClients(cfg.Kubeconfig, cfg.Cluster, cfg.TargetNamespace)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Kubernetes clients")

		// Seed store so that store.GetCRNames() returns the right names for all
		// operator tests. The TektonConfig CR is always named "config".
		store.SetCRNames(utils.ResourceNames{TektonConfig: "config"})
	},
)

var _ = AfterSuite(func() {
	hooks.CleanupNamespaces()
	_ = config.RemoveTempDir()
})

// Automatically create namespace per Describe block
var _ = hooks.AutoNamespacePerDescribe(&lastNamespace, func() *clients.Clients { return sharedClients })
