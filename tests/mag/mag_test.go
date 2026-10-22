package mag_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	approvalgate "github.com/openshift-pipelines/release-tests-ginkgo/pkg/manualapprovalgate"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

// PIPELINES-28 exercises the basic approve/reject flow of the Manual Approval Gate.
// The "approvalgate-users" label makes the suite grant user1 the
// manual-approval-gate-approver RoleBinding in the test namespace (see suite_test.go).
var _ = Describe("Manual Approval Gate: PIPELINES-28", Ordered, ContinueOnFailure, Label("approvalgate", "approvalgate-users", "mag-group-user", "e2e", "admin", "sanity", "taskrun"), func() {

	Describe("Approve Manual Approval gate pipeline: PIPELINES-28-TC01", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("validates MAG deployment is ready", func() {
			approvalgate.ValidateMAGDeployment(sharedClients)
		})
		It("ensures group1 has user1", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("creates the approval pipelinerun", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC01", "Approve Manual Approval gate pipeline",
				[]string{"group:" + group1}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves the approval task", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("validates the approval task is in Approved state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "approved", magWaitState)
		})
		It("verifies the pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Reject Manual Approval gate pipeline: PIPELINES-28-TC02", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("creates the approval pipelinerun", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC02", "Reject Manual Approval gate pipeline",
				[]string{"group:" + group1}, 1, magTimeoutFailFast, lastNamespace)
		})
		It("user1 rejects the approval task", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "reject", taskName, lastNamespace, "")
		})
		It("validates the approval task is in Rejected state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "rejected", magWaitState)
		})
		It("verifies the pipelinerun failed", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})
})
