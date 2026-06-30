package metrics_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/monitoring"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

var oc = occmd.OC{}

var _ = Describe("OpenCensus to OpenTelemetry Migration", Serial, Ordered, Label("metrics", "e2e", "admin"), func() {

	var ns string

	BeforeAll(func() {
		ns = fmt.Sprintf("otel-metrics-%d", time.Now().Unix())
		lastNamespace = ns
		oc.CreateNewProject(ns)
		sharedClients.NewClientSet(ns)
		operator.AssertServiceAccountPresent(sharedClients, ns, "pipeline")

		oc.Create("testdata/metrics/pipelinerun.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "otel-metrics-test", "successful", ns)

		oc.Create("testdata/metrics/taskrun.yaml", ns)
		pipelines.ValidateTaskRun(sharedClients, "otel-metrics-taskrun-test", "successful", ns)

		DeferCleanup(func() {
			oc.DeleteProjectIgnoreErrors(ns)
		})
	})

	BeforeEach(func() {
		lastNamespace = ns
	})

	Context("Pipeline core metrics are preserved after migration", func() {

		DescribeTable("verifies core metric exists in Prometheus",
			func(metricName string) {
				err := monitoring.VerifyMetricExists(sharedClients, metricName)
				Expect(err).NotTo(HaveOccurred(), "core metric %q should exist after OTel migration", metricName)
			},
			Entry("pipelinerun total count", "tekton_pipelines_controller_pipelinerun_total"),
			Entry("pipelinerun duration", "tekton_pipelines_controller_pipelinerun_duration_seconds_sum"),
			Entry("running pipelineruns gauge", "tekton_pipelines_controller_running_pipelineruns"),
			Entry("pipelineruns waiting on pipeline resolution", "tekton_pipelines_controller_running_pipelineruns_waiting_on_pipeline_resolution"),
			Entry("pipelineruns waiting on task resolution", "tekton_pipelines_controller_running_pipelineruns_waiting_on_task_resolution"),
			Entry("pipelinerun taskrun duration", "tekton_pipelines_controller_pipelinerun_taskrun_duration_seconds_sum"),
			Entry("taskrun total count", "tekton_pipelines_controller_taskrun_total"),
			Entry("taskrun duration", "tekton_pipelines_controller_taskrun_duration_seconds_sum"),
			Entry("running taskruns gauge", "tekton_pipelines_controller_running_taskruns"),
			Entry("taskrun pod scheduling latency", "tekton_pipelines_controller_taskruns_pod_latency_milliseconds_count"),
			// Note: running_taskruns_throttled_by_quota and _by_node are only emitted
			// when TaskRuns are actively throttled. They won't appear on clusters
			// without ResourceQuotas or node constraints.
		)
	})

	Context("Infrastructure metrics are renamed to OTel conventions", func() {

		DescribeTable("verifies new infrastructure metric name exists",
			func(metricName string) {
				err := monitoring.VerifyMetricExists(sharedClients, metricName)
				Expect(err).NotTo(HaveOccurred(), "post-migration metric %q should exist", metricName)
			},
			Entry("workqueue depth", "kn_workqueue_depth"),
			Entry("workqueue adds total", "kn_workqueue_adds_total"),
			Entry("workqueue queue duration", "kn_workqueue_queue_duration_seconds_sum"),
			Entry("workqueue process duration", "kn_workqueue_process_duration_seconds_sum"),
			Entry("workqueue retries total", "kn_workqueue_retries_total"),
			Entry("workqueue unfinished work", "kn_workqueue_unfinished_work_seconds"),
			Entry("K8s API request duration", "http_client_request_duration_seconds_sum"),
			Entry("K8s API response status codes", "kn_k8s_client_http_response_status_code_total"),
			Entry("Go runtime memory alloc", "go_memstats_alloc_bytes"),
			Entry("Go goroutines", "go_goroutines"),
		)

		DescribeTable("verifies old infrastructure metric name is absent",
			func(metricName string) {
				err := monitoring.VerifyMetricAbsent(sharedClients, metricName)
				Expect(err).NotTo(HaveOccurred(), "pre-migration metric %q should not exist after OTel migration", metricName)
			},
			Entry("old workqueue depth", "tekton_pipelines_controller_workqueue_depth"),
			Entry("old workqueue adds", "tekton_pipelines_controller_workqueue_adds_total"),
			Entry("old workqueue queue latency", "tekton_pipelines_controller_workqueue_queue_latency_seconds_sum"),
			Entry("old workqueue work duration", "tekton_pipelines_controller_workqueue_work_duration_seconds_sum"),
			Entry("old K8s client latency", "tekton_pipelines_controller_client_latency_sum"),
			Entry("old K8s client results", "tekton_pipelines_controller_client_results"),
			Entry("old Go alloc", "tekton_pipelines_controller_go_alloc"),
			Entry("old Go sys", "tekton_pipelines_controller_go_sys"),
		)
	})

	Context("Removed metrics are absent", func() {

		DescribeTable("verifies removed metric is absent",
			func(metricName string) {
				err := monitoring.VerifyMetricAbsent(sharedClients, metricName)
				Expect(err).NotTo(HaveOccurred(), "removed metric %q should not exist", metricName)
			},
			Entry("reconcile count", "tekton_pipelines_controller_reconcile_count"),
			Entry("reconcile latency", "tekton_pipelines_controller_reconcile_latency_sum"),
		)
	})

	Context("config-observability ConfigMap uses OTel keys", func() {

		It("has metrics-protocol key in active config", func() {
			err := monitoring.VerifyConfigMapKeyExists(sharedClients, config.TargetNamespace, "config-observability", "metrics-protocol")
			Expect(err).NotTo(HaveOccurred(), "config-observability should have 'metrics-protocol' key")
		})

		It("does not have legacy metrics.backend-destination key", func() {
			err := monitoring.VerifyConfigMapKeyAbsent(sharedClients, config.TargetNamespace, "config-observability", "metrics.backend-destination")
			Expect(err).NotTo(HaveOccurred(), "config-observability should not have legacy 'metrics.backend-destination' key")
		})
	})

	Context("Cross-component controllers emit OTel metrics", func() {

		DescribeTable("verifies component scrape target is healthy",
			func(jobName string) {
				err := monitoring.VerifyHealthStatusMetric(sharedClients, monitoring.TargetService{
					Job:           jobName,
					ExpectedValue: "1",
				})
				Expect(err).NotTo(HaveOccurred(), "scrape target %q should be up", jobName)
			},
			Entry("pipelines controller", "tekton-pipelines-controller"),
			Entry("triggers controller", "tekton-triggers-controller"),
			Entry("chains controller", "tekton-chains-controller"),
		)
	})
})

var _ = Describe("OpenTelemetry Migration - Triggers", Serial, Ordered, Label("metrics", "e2e", "admin"), func() {

	BeforeEach(func() {
		lastNamespace = config.TargetNamespace
	})

	Context("Triggers controller gauges are preserved after migration", func() {

		DescribeTable("verifies controller gauge exists in Prometheus",
			func(metricName string) {
				err := monitoring.VerifyMetricExists(sharedClients, metricName)
				Expect(err).NotTo(HaveOccurred(), "triggers controller gauge %q should exist", metricName)
			},
			Entry("eventlistener count", "controller_eventlistener_count"),
			Entry("triggerbinding count", "controller_triggerbinding_count"),
			Entry("clustertriggerbinding count", "controller_clustertriggerbinding_count"),
			Entry("triggertemplate count", "controller_triggertemplate_count"),
			Entry("clusterinterceptor count", "controller_clusterinterceptor_count"),
		)
	})

	Context("Triggers infrastructure metrics renamed to OTel conventions", func() {

		DescribeTable("verifies new Triggers infra metric exists",
			func(query string) {
				err := monitoring.VerifyMetricExists(sharedClients, query)
				Expect(err).NotTo(HaveOccurred(), "post-migration triggers metric %q should exist", query)
			},
			Entry("workqueue depth", `kn_workqueue_depth{job="tekton-triggers-controller"}`),
			Entry("workqueue adds total", `kn_workqueue_adds_total{job="tekton-triggers-controller"}`),
			Entry("workqueue queue duration", `kn_workqueue_queue_duration_seconds_count{job="tekton-triggers-controller"}`),
			Entry("workqueue process duration", `kn_workqueue_process_duration_seconds_count{job="tekton-triggers-controller"}`),
			Entry("K8s API request duration", `http_client_request_duration_seconds_count{job="tekton-triggers-controller"}`),
			Entry("K8s API response status codes", `kn_k8s_client_http_response_status_code_total{job="tekton-triggers-controller"}`),
		)

		DescribeTable("verifies old Triggers infra metric is absent",
			func(query string) {
				err := monitoring.VerifyMetricAbsent(sharedClients, query)
				Expect(err).NotTo(HaveOccurred(), "pre-migration triggers metric %q should not exist", query)
			},
			Entry("old workqueue depth", `controller_workqueue_depth{job="tekton-triggers-controller"}`),
			Entry("old workqueue adds", `controller_workqueue_adds_total{job="tekton-triggers-controller"}`),
			Entry("old K8s client latency", `controller_client_latency_count{job="tekton-triggers-controller"}`),
			Entry("old K8s client results", `controller_client_results{job="tekton-triggers-controller"}`),
			Entry("old Go alloc", `controller_go_alloc{job="tekton-triggers-controller"}`),
			Entry("old reconcile count", `controller_reconcile_count{job="tekton-triggers-controller"}`),
		)
	})
})
