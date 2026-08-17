package results_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var oc = occmd.OC{}
var _ = Describe("Tekton Results: PIPELINES-26", Label("results", "e2e"), func() {

	Describe("Test Tekton results with TaskRun: PIPELINES-26-TC01", Ordered, ContinueOnFailure, func() {
		It("verifies golang imagestream exists", func() {
			cmd.MustSucceed("oc", "get", "is", "golang", "-n", "openshift")
		})

		It("applies taskrun fixture and verifies completion", func() {
			oc.Apply("testdata/results/taskrun.yaml")

			// Wait for taskrun to complete (namespace from store)
			ns := store.Namespace()
			cmd.MustSucceedIncreasedTimeout(time.Minute*5, "oc", "wait", "--for=condition=Succeeded", "taskrun/results-task", "-n", ns, "--timeout=120s")
		})

		It("verifies taskrun results are stored", func() {
			err := operator.VerifyResultsAnnotationStored(sharedClients, "taskrun")
			Expect(err).NotTo(HaveOccurred(), "Results annotation not stored for taskrun")
		})

		It("verifies taskrun results records", func() {
			err := operator.VerifyResultsRecords("taskrun")
			Expect(err).NotTo(HaveOccurred(), "Results records verification failed for taskrun")
		})

		It("verifies taskrun results logs", func() {
			err := operator.VerifyResultsLogs("taskrun")
			Expect(err).NotTo(HaveOccurred(), "Results logs verification failed for taskrun")
		})
	})

	Describe("Test Tekton results with PipelineRun: PIPELINES-26-TC02", Label("sanity"), Ordered, ContinueOnFailure, func() {
		It("verifies golang imagestream exists", func() {
			cmd.MustSucceed("oc", "get", "is", "golang", "-n", "openshift")
		})

		It("applies pipeline and pipelinerun fixtures", func() {
			oc.Apply("testdata/results/pipeline.yaml")
			oc.Apply("testdata/results/pipelinerun.yaml")

			// Wait for pipelinerun to complete
			ns := store.Namespace()
			cmd.MustSucceedIncreasedTimeout(time.Minute*5, "oc", "wait", "--for=condition=Succeeded", "pipelinerun/pipeline-results", "-n", ns, "--timeout=120s")
		})

		It("verifies pipelinerun results are stored", func() {
			err := operator.VerifyResultsAnnotationStored(sharedClients, "pipelinerun")
			Expect(err).NotTo(HaveOccurred(), "Results annotation not stored for pipelinerun")
		})

		It("verifies pipelinerun results records", func() {
			err := operator.VerifyResultsRecords("pipelinerun")
			Expect(err).NotTo(HaveOccurred(), "Results records verification failed for pipelinerun")
		})

		It("verifies pipelinerun results logs", func() {
			err := operator.VerifyResultsLogs("pipelinerun")
			Expect(err).NotTo(HaveOccurred(), "Results logs verification failed for pipelinerun")
		})
	})
})
