package mag_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/hooks"
	approvalgate "github.com/openshift-pipelines/release-tests-ginkgo/pkg/manualapprovalgate"
)

var sharedClients *clients.Clients

var lastNamespace string

func TestMAG(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MAG Suite", Label("mag"))
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
		_ = cs

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

var _ = hooks.AutoNamespacePerDescribe(&lastNamespace, func() *clients.Clients { return sharedClients })

// After SRVKP-12172, regular users no longer inherit cluster-wide ApprovalTask access.
// For approvalgate-users specs, bind user1–user5 to manual-approval-gate-approver in
// each dynamically created releasetest-* namespace (including retry namespaces).
var _ = BeforeEach(func() {
	if !specHasLabel("approvalgate-users") || lastNamespace == "" {
		return
	}
	approvalgate.EnsureApproverRoleBindings(lastNamespace)
})

func specHasLabel(label string) bool {
	for _, labels := range CurrentSpecReport().ContainerHierarchyLabels {
		for _, l := range labels {
			if l == label {
				return true
			}
		}
	}
	return false
}

var _ = AfterSuite(func() {
	hooks.CleanupNamespaces()
	approvalgate.CleanupUserKubeconfigs()
	_ = config.RemoveTempDir()
})
