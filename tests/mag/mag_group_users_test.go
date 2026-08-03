package mag_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	approvalgate "github.com/openshift-pipelines/release-tests-ginkgo/pkg/manualapprovalgate"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

const (
	magTimeoutSuccess  = "5m"
	magTimeoutFailFast = "2m"
	magTimeoutShort    = "30s"
	magWaitState       = 2 * time.Minute
	magWaitMessage     = 60 * time.Second
)

var _ = Describe("Manual Approval Gate Group Users: PIPELINES-37", Ordered, ContinueOnFailure, Label("approvalgate", "approvalgate-users", "mag-group-user", "e2e", "taskrun", "sanity", "admin"), func() {

	Describe("Single Group / Single Approval: PIPELINES-37-TC01", Ordered, func() {
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
				sharedClients, "TC01", "Single Group / Single Approval",
				[]string{"group:" + group1}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves the task", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("validates Approved state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "approved", magWaitState)
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Quorum: Partial to Complete: PIPELINES-37-TC02", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1 and user2", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2"})
		})
		It("creates the approval pipelinerun requiring 2 approvals", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC02", "Quorum: Partial to Complete",
				[]string{"group:" + group1}, 2, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("validates task: 2 required, 1 pending, 0 rejected, Pending status", func() {
			approvalgate.WaitForAndAssertApprovalTaskListState(sharedClients, taskName, 2, 1, 0, "Pending", magWaitState)
		})
		It("user2 approves", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user2", "approve", taskName, lastNamespace, "")
		})
		It("validates task: 2 required, 0 pending, 0 rejected, Approved status", func() {
			approvalgate.WaitForAndAssertApprovalTaskListState(sharedClients, taskName, 2, 0, 0, "Approved", magWaitState)
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Mixed Entities: User + Group: PIPELINES-37-TC03", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1 and user2", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2"})
		})
		It("creates the approval pipelinerun with user5 and group1", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC03", "Mixed Entities: User + Group",
				[]string{"user5", "group:" + group1}, 2, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves via group", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("user5 approves directly", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user5", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Rejection Authority: PIPELINES-37-TC04", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1 and user2", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2"})
		})
		It("creates the approval pipelinerun requiring 2 approvals", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC04", "Rejection Authority",
				[]string{"group:" + group1}, 2, magTimeoutFailFast, lastNamespace)
		})
		It("user1 approves", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("user2 rejects", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user2", "reject", taskName, lastNamespace, "")
		})
		It("validates Rejected state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "rejected", magWaitState)
		})
		It("verifies pipelinerun failed", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("Change of Mind: Approve then Reject: PIPELINES-37-TC05", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1 and user2", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2"})
		})
		It("creates the approval pipelinerun requiring 2 approvals", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC05", "Change of Mind: Approve then Reject",
				[]string{"group:" + group1}, 2, magTimeoutFailFast, lastNamespace)
		})
		It("user1 approves initially", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("user1 changes mind and rejects", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "reject", taskName, lastNamespace, "")
		})
		It("validates Rejected state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "rejected", magWaitState)
		})
		It("verifies pipelinerun failed", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("Non-Member Block: PIPELINES-37-TC06", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1 only", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("creates the approval pipelinerun", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC06", "Non-Member Block",
				[]string{"group:" + group1}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("user4 (non-member) approval attempt fails", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user4", "approve-expect-fail", taskName, lastNamespace, "")
		})
		It("user1 (member) approves successfully", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Multi-Group Consensus: PIPELINES-37-TC07", Ordered, func() {
		var group1, group2, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			group2 = approvalgate.MAGGroupName(lastNamespace, "group2")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
			approvalgate.DeleteGroup(group2)
		})

		It("ensures group1 has user1", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("ensures group2 has user4", func() {
			approvalgate.EnsureGroupMembers(group2, []string{"user4"})
		})
		It("creates the approval pipelinerun requiring both groups", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC07", "Multi-Group Consensus",
				[]string{"group:" + group1, "group:" + group2}, 2, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves via group1", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("user4 approves via group2", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user4", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Overlapping Group Membership: PIPELINES-37-TC08", Ordered, func() {
		var group1, group2, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			group2 = approvalgate.MAGGroupName(lastNamespace, "group2")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
			approvalgate.DeleteGroup(group2)
		})

		It("ensures group1 has user1 and user2", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2"})
		})
		It("ensures group2 has user2 only", func() {
			approvalgate.EnsureGroupMembers(group2, []string{"user2"})
		})
		It("creates the approval pipelinerun requiring 2 approvals across both groups", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC08", "Overlapping Group Membership",
				[]string{"group:" + group1, "group:" + group2}, 2, magTimeoutSuccess, lastNamespace)
		})
		It("user2 approves (member of both groups)", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user2", "approve", taskName, lastNamespace, "")
		})
		It("user1 approves (member of group1)", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Timeout Expiry with Short Timeout: PIPELINES-37-TC09", Ordered, func() {
		var group1, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1 and user2", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2"})
		})
		It("creates the approval pipelinerun with 30s timeout requiring 2 approvals", func() {
			prName, _ = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC09", "Timeout Expiry with Short Timeout",
				[]string{"group:" + group1}, 2, magTimeoutShort, lastNamespace)
		})
		It("verifies pipelinerun failed after timeout", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("Multi-Group Race: Any One Can Approve: PIPELINES-37-TC10", Ordered, func() {
		var group1, group2, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			group2 = approvalgate.MAGGroupName(lastNamespace, "group2")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
			approvalgate.DeleteGroup(group2)
		})

		It("ensures group1 has user1", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("ensures group2 has user5", func() {
			approvalgate.EnsureGroupMembers(group2, []string{"user5"})
		})
		It("creates the approval pipelinerun requiring 1 approval from any group", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC10", "Multi-Group Race: Any One Can Approve",
				[]string{"group:" + group1, "group:" + group2}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("user5 approves via group2", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user5", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Mixed Entity Change-of-Mind: PIPELINES-37-TC11", Ordered, func() {
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
		It("creates the approval pipelinerun with user1 as both direct and group approver", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC11", "Mixed Entity Change-of-Mind",
				[]string{"user1", "group:" + group1}, 2, magTimeoutFailFast, lastNamespace)
		})
		It("user1 approves initially", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("user1 changes mind and rejects", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "reject", taskName, lastNamespace, "")
		})
		It("validates Rejected state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "rejected", magWaitState)
		})
		It("verifies pipelinerun failed", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("Re-approve Completed Task: PIPELINES-37-TC12", Ordered, func() {
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
				sharedClients, "TC12", "Re-approve Completed Task",
				[]string{"group:" + group1}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves the task", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("validates Approved state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "approved", magWaitState)
		})
		It("user1 re-approval attempt fails on completed task", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve-expect-fail", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Impossible Quorum with Short Timeout: PIPELINES-37-TC13", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 has user1, user2, and user3", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2", "user3"})
		})
		It("creates the approval pipelinerun requiring 4 approvals from 3-member group with short timeout", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC13", "Impossible Quorum with Short Timeout",
				[]string{"group:" + group1}, 4, magTimeoutShort, lastNamespace)
		})
		It("user1 approves", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("user2 approves (allows final state)", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user2", "approve-allow-final-state", taskName, lastNamespace, "")
		})
		It("user3 approves (allows final state)", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user3", "approve-allow-final-state", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun failed due to impossible quorum timeout", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("Invalid Group Name: PIPELINES-37-TC14", Ordered, func() {
		var invalidGroup, taskName, prName string

		BeforeAll(func() {
			invalidGroup = approvalgate.MAGGroupName(lastNamespace, "invalid-group")
			sharedClients.NewClientSet(lastNamespace)
		})

		It("creates the approval pipelinerun with a non-existent group", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC14", "Invalid Group Name",
				[]string{"group:" + invalidGroup}, 1, magTimeoutShort, lastNamespace)
		})
		It("user1 approval attempt fails (not a member)", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve-expect-fail", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun failed after timeout", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("Re-approve Rejected Task: PIPELINES-37-TC15", Ordered, func() {
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
				sharedClients, "TC15", "Re-approve Rejected Task",
				[]string{"group:" + group1}, 1, magTimeoutFailFast, lastNamespace)
		})
		It("user1 rejects the task", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "reject", taskName, lastNamespace, "")
		})
		It("validates Rejected state", func() {
			approvalgate.WaitForApprovalTaskState(sharedClients, taskName, "rejected", magWaitState)
		})
		It("user1 re-approval attempt fails on rejected task", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve-expect-fail", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun failed", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("The Late Joiner: User Added After PR Creation: PIPELINES-37-TC16", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 starts empty", func() {
			approvalgate.EnsureGroupMembers(group1, []string{})
		})
		It("creates the approval pipelinerun with empty group", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC16", "The Late Joiner",
				[]string{"group:" + group1}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("user1 joins the group after PR creation", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("user1 approves as a late joiner", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("The Evicted User: Removed from Group After PR Creation: PIPELINES-37-TC17", Ordered, func() {
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
		It("creates the approval pipelinerun with short timeout", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC17", "The Evicted User",
				[]string{"group:" + group1}, 1, magTimeoutShort, lastNamespace)
		})
		It("user1 is evicted from the group", func() {
			approvalgate.EnsureGroupMembers(group1, []string{})
		})
		It("user1 approval attempt fails after eviction", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve-expect-fail", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun failed", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "failed", lastNamespace)
		})
	})

	Describe("The Switcheroo: Group Membership Replaced: PIPELINES-37-TC18", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 initially has user1", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("creates the approval pipelinerun", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC18", "The Switcheroo",
				[]string{"group:" + group1}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("group membership is switched from user1 to user2", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user2"})
		})
		It("user1 approval attempt fails after removal", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve-expect-fail", taskName, lastNamespace, "")
		})
		It("user2 approves as new member", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user2", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Dynamic Quorum: Member Added to Meet Quorum: PIPELINES-37-TC19", Ordered, func() {
		var group1, taskName, prName string

		BeforeAll(func() {
			group1 = approvalgate.MAGGroupName(lastNamespace, "group1")
			sharedClients.NewClientSet(lastNamespace)
		})
		AfterAll(func() {
			approvalgate.DeleteGroup(group1)
		})

		It("ensures group1 initially has user1 only", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1"})
		})
		It("creates the approval pipelinerun requiring 2 approvals", func() {
			prName, taskName = approvalgate.CreateApprovalPipelineRun(
				sharedClients, "TC19", "Dynamic Quorum",
				[]string{"group:" + group1}, 2, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves (one approval short)", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, "")
		})
		It("user2 is added to group1 dynamically", func() {
			approvalgate.EnsureGroupMembers(group1, []string{"user1", "user2"})
		})
		It("user2 approves to complete quorum", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user2", "approve", taskName, lastNamespace, "")
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})

	Describe("Approval Message Audit: PIPELINES-37-TC20", Ordered, func() {
		var group1, taskName, prName string
		const approvalMsg = "PIPELINES-37-TC20 custom approve message from user1"

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
				sharedClients, "TC20", "Approval Message Audit",
				[]string{"group:" + group1}, 1, magTimeoutSuccess, lastNamespace)
		})
		It("user1 approves with a custom message", func() {
			approvalgate.PerformApprovalTaskActionAsUser("user1", "approve", taskName, lastNamespace, approvalMsg)
		})
		It("verifies the approval message is recorded in the task", func() {
			approvalgate.WaitForApprovalTaskMessageContains(sharedClients, taskName, approvalMsg, magWaitMessage)
		})
		It("verifies pipelinerun succeeded", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})
	})
})
