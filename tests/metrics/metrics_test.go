package metrics_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/monitoring"
)

var _ = Describe("OpenShift Pipelines Monitoring: PIPELINES-01", Label("metrics", "e2e", "admin", "sanity"), func() {

	Describe("OpenShift pipelines metrics acceptance tests: PIPELINES-01-TC01", Ordered, ContinueOnFailure, func() {

		DescribeTable("verifies job health status metrics",
			func(jobName, expectedValue string) {
				err := monitoring.VerifyHealthStatusMetric(sharedClients, monitoring.TargetService{
					Job:           jobName,
					ExpectedValue: expectedValue,
				})
				Expect(err).NotTo(HaveOccurred(), "Health status metric check failed for job: %s", jobName)
			},
			Entry("node-exporter is healthy", "node-exporter", "1"),
			Entry("kube-state-metrics is healthy", "kube-state-metrics", "1"),
			Entry("prometheus-k8s is healthy", "prometheus-k8s", "1"),
			Entry("prometheus-operator is healthy", "prometheus-operator", "1"),
			Entry("alertmanager-main is healthy", "alertmanager-main", "1"),
			Entry("tekton-pipelines-controller is healthy", "tekton-pipelines-controller", "1"),
		)

		It("verifies pipelines control plane metrics", func() {
			err := monitoring.VerifyPipelinesControlPlaneMetrics(sharedClients)
			Expect(err).NotTo(HaveOccurred(), "Control plane metrics verification failed")
		})
	})
})
