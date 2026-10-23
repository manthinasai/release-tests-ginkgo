package tektonkueue_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	olmpkg "github.com/openshift-pipelines/release-tests-ginkgo/pkg/olm"
)

var _ = Describe("Multi-cluster operator bootstrap: PIPELINES-41", Serial, Label("tekton-kueue", "install", "admin"), func() {
	It("installs the required operators on the hub and every spoke: PIPELINES-41-TC01", NodeTimeout(6*time.Hour), func(specCtx SpecContext) {
		type clusterTarget struct {
			name   string
			config clientConfig
		}
		clusters := make([]clusterTarget, 0, 1+len(spokeConfigs))
		clusters = append(clusters, clusterTarget{name: "hub", config: hubConfig})
		for i, cfg := range spokeConfigs {
			clusters = append(clusters, clusterTarget{name: fmt.Sprintf("spoke-%d", i+1), config: cfg})
		}

		for _, cluster := range clusters {
			By(fmt.Sprintf("bootstrapping %s", cluster.name))
			controllerClient, err := sharedClients.NewClientFromKubeconfig(cluster.config.Kubeconfig, cluster.config.Cluster, cluster.config.Context)
			Expect(err).NotTo(HaveOccurred(), "failed to create %s client", cluster.name)

			bootstrap := olmpkg.ClusterBootstrap{Client: controllerClient}
			Expect(bootstrap.EnsureOperators(specCtx)).To(Succeed(), "failed to bootstrap %s", cluster.name)
		}
	})
})
