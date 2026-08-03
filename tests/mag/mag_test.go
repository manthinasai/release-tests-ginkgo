package mag_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	approvalgate "github.com/openshift-pipelines/release-tests-ginkgo/pkg/manualapprovalgate"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

var oc = occmd.OC{}

var _ = Describe("Manual Approval Gate: PIPELINES-28", Ordered, ContinueOnFailure, Label("approvalgate", "e2e", "admin", "sanity", "taskrun"), func() {

	Describe("Approve Manual Approval gate pipeline: PIPELINES-28-TC01", Ordered, func() {
		It("validates MAG deployment is ready", func() {
			approvalgate.ValidateMAGDeployment(sharedClients)
		})

		It("creates the manual approval pipeline", func() {
			oc.Create("testdata/manualapprovalgate/manual-approval-pipeline.yaml", lastNamespace)
		})

		It("starts the pipeline with workspace", func() {
			cmd.MustSucceed("opc", "pipeline", "start", "manual-approval-pipeline",
				"-n", lastNamespace)
		})

		It("approves the approval task", func() {
			tasks, err := approvalgate.ListApprovalTask(sharedClients)
			Expect(err).NotTo(HaveOccurred(), "Failed to list approval tasks")
			Expect(tasks).NotTo(BeEmpty(), "No approval tasks found")
			approvalgate.ApproveApprovalGatePipeline(tasks[0].Name)
		})

		It("validates pipeline is in Approved state", func() {
			result, err := approvalgate.ValidateApprovalGatePipeline(sharedClients, "Approved")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue(), "Pipeline not in Approved state")
		})

		It("verifies the latest pipelinerun succeeded", func() {
			prName, err := pipelines.GetLatestPipelinerun(sharedClients, lastNamespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to get latest pipelinerun")
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Reject Manual Approval gate pipeline: PIPELINES-28-TC02", Ordered, func() {
		It("creates the manual approval pipeline", func() {
			oc.Create("testdata/manualapprovalgate/manual-approval-pipeline.yaml", lastNamespace)
		})

		It("starts the pipeline with workspace", func() {
			cmd.MustSucceed("opc", "pipeline", "start", "manual-approval-pipeline",
				"-n", lastNamespace)
		})

		It("rejects the approval task", func() {
			tasks, err := approvalgate.ListApprovalTask(sharedClients)
			Expect(err).NotTo(HaveOccurred(), "Failed to list approval tasks")
			Expect(tasks).NotTo(BeEmpty(), "No approval tasks found")
			approvalgate.RejectApprovalGatePipeline(tasks[0].Name)
		})

		It("validates pipeline is in Rejected state", func() {
			result, err := approvalgate.ValidateApprovalGatePipeline(sharedClients, "Rejected")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue(), "Pipeline not in Rejected state")
		})

		It("verifies the latest pipelinerun failed", func() {
			prName, err := pipelines.GetLatestPipelinerun(sharedClients, lastNamespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to get latest pipelinerun")
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})
})
