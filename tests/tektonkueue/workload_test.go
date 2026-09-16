package tektonkueue_test

import (
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	olmpkg "github.com/openshift-pipelines/release-tests-ginkgo/pkg/olm"
	workload "github.com/openshift-pipelines/release-tests-ginkgo/pkg/tektonkueue"
)

var _ = Describe("Multi-cluster PipelineRun execution", Serial, Label("tekton-kueue", "workload", "admin"), func() {
	It("executes a hub PipelineRun on exactly one spoke and propagates completion", NodeTimeout(2*time.Hour), func(specCtx SpecContext) {
		prefix := "rtg-mk-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		type target struct {
			name   string
			config clientConfig
		}
		targets := make([]target, 0, 1+len(spokeConfigs))
		targets = append(targets, target{name: "hub", config: hubConfig})
		for i, cfg := range spokeConfigs {
			targets = append(targets, target{name: fmt.Sprintf("spoke-%d", i+1), config: cfg})
		}

		configured := make([]workload.Cluster, 0, len(targets))
		for _, target := range targets {
			clusterClients, err := clients.NewClientsWithContext(
				target.config.Kubeconfig,
				target.config.Cluster,
				target.config.Context,
				prefix,
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create clients for %s", target.name)

			controllerClient, err := clusterClients.NewClientFromKubeconfig(
				target.config.Kubeconfig,
				target.config.Cluster,
				target.config.Context,
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create bootstrap client for %s", target.name)
			Expect((&olmpkg.ClusterBootstrap{Client: controllerClient}).EnsureOperators(specCtx)).To(Succeed(),
				"failed to bootstrap %s", target.name)
			configured = append(configured, workload.Cluster{Name: target.name, Clients: clusterClients})
		}

		environment := workload.NewEnvironment(configured[0], configured[1:], prefix)
		DeferCleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			if err := environment.Cleanup(cleanupCtx); err != nil {
				GinkgoWriter.Printf("retrying failed multi-cluster cleanup: %v\n", err)
				Expect(environment.Cleanup(cleanupCtx)).To(Succeed(), "failed to restore multi-cluster test environment")
			}
		})

		Expect(environment.Setup(specCtx)).To(Succeed(), "failed to configure multi-cluster workload environment")
		result, err := environment.Execute(specCtx)
		Expect(err).NotTo(HaveOccurred(), "multi-cluster PipelineRun execution failed")
		GinkgoWriter.Printf("PipelineRun %s executed on %s and completed on the hub\n", result.PipelineRun, result.Spoke)
	})
})
