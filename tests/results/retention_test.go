package results_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	approvalgate "github.com/openshift-pipelines/release-tests-ginkgo/pkg/manualapprovalgate"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Results Retention Policy: PIPELINES-26", Ordered, ContinueOnFailure, Label("results", "retention", "e2e"), func() {

	// Ensure retention policy is restored even if tests are interrupted
	BeforeAll(func() {
		DeferCleanup(func() {
			By("Restoring default retention policy (30d, weekly)")
			err := operator.ConfigureRetentionPolicy("5 5 * * 0", "30d", "", "")
			if err != nil {
				GinkgoLogr.Error(err, "Failed to restore default retention policy")
			}
		})
	})

	// Group 1: Default Retention (Fallback)

	Describe("TC03: defaultRetention (Primary Fallback): PIPELINES-26-TC03", Label("sanity"), Ordered, func() {
		var prName string

		It("configures retention policy with defaultRetention", func() {
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "1m", "", "")
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs a PipelineRun and waits for completion", func() {
			oc.Apply("testdata/results/pipeline.yaml")
			oc.Apply("testdata/results/pipelinerun.yaml")
			var err error
			prName, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", store.Namespace())
		})

		It("verifies result is stored in Results API", func() {
			err := operator.VerifyResultsAnnotationStored(sharedClients, "pipelinerun")
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultExists("pipelinerun", prName, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
		})

		It("waits for retention period and verifies result is deleted", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", prName, store.Namespace(), 150*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("TC04: maxRetention (Backward Compatibility): PIPELINES-26-TC04", Ordered, func() {
		var prName string

		It("configures retention policy with maxRetention only", func() {
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "", "1m", "")
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs a PipelineRun and waits for completion", func() {
			oc.Apply("testdata/results/pipeline.yaml")
			oc.Apply("testdata/results/pipelinerun.yaml")
			var err error
			prName, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", store.Namespace())
		})

		It("verifies result is stored in Results API", func() {
			err := operator.VerifyResultsAnnotationStored(sharedClients, "pipelinerun")
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultExists("pipelinerun", prName, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
		})

		It("waits for retention period and verifies result is deleted", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", prName, store.Namespace(), 150*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// Group 2: Single-Selector Policies

	Describe("TC05: matchNamespaces: PIPELINES-26-TC05", Ordered, func() {
		var testNS string
		var defaultNS string

		BeforeAll(func() {
			testNS = "retention-test-ns"
			oc.CreateNewProject(testNS)
			k8s.WaitForServiceAccount(sharedClients, testNS, "pipeline")
			DeferCleanup(func() { oc.DeleteProjectIgnoreErrors(testNS) })
		})

		It("configures retention policy with matchNamespaces", func() {
			policy := fmt.Sprintf(`    - name: "ns-test"
      selector:
        matchNamespaces:
          - "%s"
      retention: "1m"`, testNS)
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs PipelineRun in test-ns and default namespace", func() {
			// Run in test namespace
			oc.Apply("testdata/results/pipeline.yaml", testNS)
			oc.Apply("testdata/results/pipelinerun.yaml", testNS)
			sharedClients.NewClientSet(testNS) // Switch client to testNS
			testNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, testNS)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, testNSPRName, "successful", testNS)

			// Run in default auto-created namespace
			defaultNS = store.Namespace()
			sharedClients.NewClientSet(defaultNS) // Switch client to defaultNS
			oc.Apply("testdata/results/pipeline.yaml", defaultNS)
			oc.Apply("testdata/results/pipelinerun.yaml", defaultNS)
			defaultNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, defaultNS)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, defaultNSPRName, "successful", defaultNS)
		})

		It("waits and verifies test-ns result is deleted, default is kept", func() {
			testNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, testNS)
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultDeletedWithTimeout("pipelinerun", testNSPRName, testNS, 90*time.Second)
			Expect(err).NotTo(HaveOccurred())

			defaultNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, defaultNS)
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultExists("pipelinerun", defaultNSPRName, defaultNS)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("TC06: matchLabels: PIPELINES-26-TC06", Ordered, func() {
		var prName string

		It("configures retention policy with matchLabels", func() {
			policy := `    - name: "label-test"
      selector:
        matchLabels:
          "env":
            - "dev"
      retention: "1m"`
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs PipelineRun with env=dev label", func() {
			oc.Apply("testdata/results/pipeline.yaml")
			oc.Apply("testdata/results/pipelinerun-env-dev.yaml")
			var err error
			prName, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", store.Namespace())
		})

		It("waits and verifies env:dev result is deleted", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", prName, store.Namespace(), 150*time.Second) // 2.5m for 1m retention + CronJob delay
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("TC07: matchAnnotations: PIPELINES-26-TC07", Ordered, func() {
		var prName string

		It("configures retention policy with matchAnnotations", func() {
			policy := `    - name: "anno-test"
      selector:
        matchAnnotations:
          "keep-for-debug":
            - "true"
      retention: "1m"`
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs PipelineRun with annotation", func() {
			oc.Apply("testdata/results/pipeline.yaml")
			oc.Apply("testdata/results/pipelinerun-keep-debug.yaml")
			var err error
			prName, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", store.Namespace())
		})

		It("waits and verifies annotated result is deleted", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", prName, store.Namespace(), 150*time.Second) // 2.5m for 1m retention + CronJob delay
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("TC08: matchStatuses (Failed): PIPELINES-26-TC08", Ordered, func() {
		var failedPR string
		var succeededPR string

		It("configures retention policy for Failed status", func() {
			policy := `    - name: "status-test"
      selector:
        matchStatuses:
          - "Failed"
      retention: "1m"`
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs a failing PipelineRun", func() {
			oc.Apply("testdata/results/pipelinerun-fail.yaml")
			var err error
			failedPR, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, failedPR, "failed", store.Namespace())
		})

		It("runs a successful PipelineRun", func() {
			oc.Apply("testdata/results/pipeline.yaml")
			oc.Apply("testdata/results/pipelinerun.yaml")
			var err error
			succeededPR, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, succeededPR, "successful", store.Namespace())
		})

		It("waits and verifies Failed PR is deleted, Succeeded PR is kept", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", failedPR, store.Namespace(), 120*time.Second)
			Expect(err).NotTo(HaveOccurred())

			err = operator.VerifyResultExists("pipelinerun", succeededPR, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("TC09: Standalone TaskRun: PIPELINES-26-TC09", Ordered, func() {
		It("configures retention policy with matchLabels for TaskRun", func() {
			policy := `    - name: "label-test"
      selector:
        matchLabels:
          "env":
            - "dev"
      retention: "1m"`
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs standalone TaskRun with label", func() {
			oc.Apply("testdata/results/taskrun-env-dev.yaml")
			pipelines.ValidateTaskRun(sharedClients, "results-task-env-dev", "successful", store.Namespace())
		})

		It("waits and verifies TaskRun result is deleted", func() {
			err := operator.VerifyResultDeletedWithTimeout("taskrun", "results-task-env-dev", store.Namespace(), 150*time.Second) // 2.5m for 1m retention + CronJob delay
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// Group 3: Complex Logic (AND / OR / Order)

	Describe("TC10: Combined Selectors (AND Logic): PIPELINES-26-TC10", Ordered, func() {
		var prodNS string
		var defaultNS string

		BeforeAll(func() {
			prodNS = "retention-production"
			oc.CreateNewProject(prodNS)
			k8s.WaitForServiceAccount(sharedClients, prodNS, "pipeline")
			DeferCleanup(func() { oc.DeleteProjectIgnoreErrors(prodNS) })
		})

		It("configures retention policy with combined selectors", func() {
			policy := fmt.Sprintf(`    - name: "prod-failures"
      selector:
        matchNamespaces:
          - "%s"
        matchLabels:
          "env":
            - "prod"
        matchStatuses:
          - "Failed"
      retention: "1m"`, prodNS)
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs matching PipelineRun (production + env:prod + Failed)", func() {
			oc.Apply("testdata/results/pipelinerun-env-prod-fail.yaml", prodNS)
			sharedClients.NewClientSet(prodNS) // Switch client to prodNS
			prodNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, prodNS)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, prodNSPRName, "failed", prodNS)
		})

		It("runs non-matching PipelineRun (wrong namespace)", func() {
			defaultNS = store.Namespace()
			sharedClients.NewClientSet(defaultNS) // Switch client back to default namespace
			// Wrong namespace (default instead of production) - should NOT match
			oc.Apply("testdata/results/pipelinerun-env-prod-fail.yaml", defaultNS)
			defaultNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, defaultNS)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, defaultNSPRName, "failed", defaultNS)
		})

		It("waits and verifies only production PR is deleted", func() {
			prodNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, prodNS)
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultDeletedWithTimeout("pipelinerun", prodNSPRName, prodNS, 120*time.Second)
			Expect(err).NotTo(HaveOccurred())

			defaultNSPRName, err := pipelines.GetLatestPipelinerun(sharedClients, defaultNS)
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultExists("pipelinerun", defaultNSPRName, defaultNS)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("TC11: Selector List (OR Logic): PIPELINES-26-TC11", Ordered, func() {
		var frontendPR string
		var backendPR string

		It("configures retention policy with OR logic", func() {
			policy := `    - name: "ci-apps"
      selector:
        matchLabels:
          "app":
            - "frontend"
            - "backend"
        matchStatuses:
          - "Succeeded"
          - "Failed"
      retention: "1m"`
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs PipelineRuns with different app labels", func() {
			oc.Apply("testdata/results/pipeline.yaml")
			// app=frontend (should match)
			oc.Apply("testdata/results/pipelinerun-app-frontend.yaml")
			var err error
			frontendPR, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, frontendPR, "successful", store.Namespace())
			// app=backend (should match)
			oc.Apply("testdata/results/pipelinerun-app-backend.yaml")
			backendPR, err = pipelines.GetLatestPipelinerun(sharedClients, store.Namespace())
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, backendPR, "successful", store.Namespace())
		})

		It("waits and verifies both matching PRs are deleted", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", frontendPR, store.Namespace(), 150*time.Second) // 2.5m for 1m retention + CronJob delay
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultDeletedWithTimeout("pipelinerun", backendPR, store.Namespace(), 30*time.Second) // Should already be deleted
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("TC12: Policy Order (First Match Wins): PIPELINES-26-TC12", Ordered, func() {
		var prodNS string
		var failedPRName string
		var succeededPRName string

		BeforeAll(func() {
			prodNS = "retention-prod-order"
			oc.CreateNewProject(prodNS)
			k8s.WaitForServiceAccount(sharedClients, prodNS, "pipeline")
			DeferCleanup(func() { oc.DeleteProjectIgnoreErrors(prodNS) })
		})

		It("configures retention policy with specific-first order", func() {
			policy := fmt.Sprintf(`    - name: "specific-prod-failure"
      selector:
        matchNamespaces:
          - "%s"
        matchStatuses:
          - "Failed"
      retention: "3m"
    - name: "all-prod"
      selector:
        matchNamespaces:
          - "%s"
      retention: "1m"`, prodNS, prodNS)
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "5m", "", policy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("runs Failed PR (matches first policy - 3m)", func() {
			oc.Apply("testdata/results/pipelinerun-fail-3m.yaml", prodNS)
			sharedClients.NewClientSet(prodNS) // Switch client to prodNS
			var err error
			failedPRName, err = pipelines.GetLatestPipelinerun(sharedClients, prodNS)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, failedPRName, "failed", prodNS)
		})

		It("runs Succeeded PR (matches second policy - 1m)", func() {
			sharedClients.NewClientSet(prodNS) // Ensure client is on prodNS
			oc.Apply("testdata/results/pipeline.yaml", prodNS)
			oc.Apply("testdata/results/pipelinerun.yaml", prodNS)
			var err error
			succeededPRName, err = pipelines.GetLatestPipelinerun(sharedClients, prodNS)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, succeededPRName, "successful", prodNS)
		})

		It("verifies Succeeded PR is deleted at 1m, Failed PR still exists", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", succeededPRName, prodNS, 150*time.Second) // 2.5m for 1m retention + CronJob delay
			Expect(err).NotTo(HaveOccurred())

			// Failed PR should still exist (3m retention)
			err = operator.VerifyResultExists("pipelinerun", failedPRName, prodNS)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// Group 4: CustomRun Support (MAG Integration)

	Describe("TC13: CustomRun Basic Retention (MAG): PIPELINES-26-TC13", Label("customrun"), Ordered, func() {
		var prName string

		It("configures retention policy with defaultRetention", func() {
			err := operator.ConfigureRetentionPolicy("*/1 * * * *", "2m", "", "")
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates and starts MAG pipeline (creates CustomRun)", func() {
			// Create the MAG pipeline (retention-specific with "user" as approver)
			oc.Apply("testdata/results/manual-approval-pipeline-retention.yaml", lastNamespace)

			// Start the pipeline (creates PipelineRun with dynamic name)
			cmd.MustSucceed("opc", "pipeline", "start", "manual-approval-pipeline-retention", "-n", lastNamespace)

			// Get the latest PipelineRun name (dynamic name created by 'pipeline start')
			var err error
			prName, err = pipelines.GetLatestPipelinerun(sharedClients, lastNamespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to get PipelineRun name")
			Expect(prName).NotTo(BeEmpty(), "PipelineRun name is empty")
		})

		It("approves the approval task as user", func() {
			tasks, err := approvalgate.ListApprovalTask(sharedClients)
			Expect(err).NotTo(HaveOccurred(), "Failed to list approval tasks")
			Expect(tasks).NotTo(BeEmpty(), "No approval tasks found")
			approvalgate.PerformApprovalTaskActionAsUser("user", "approve", tasks[0].Name, lastNamespace, "")
		})

		It("waits for PipelineRun to succeed (contains CustomRun)", func() {
			pipelines.ValidatePipelineRun(sharedClients, prName, "successful", lastNamespace)
		})

		It("verifies PipelineRun result is stored in Results API (includes CustomRun)", func() {
			err := operator.VerifyResultsAnnotationStored(sharedClients, "pipelinerun")
			Expect(err).NotTo(HaveOccurred())
			err = operator.VerifyResultExists("pipelinerun", prName, lastNamespace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("waits for retention period and verifies PipelineRun result is deleted", func() {
			err := operator.VerifyResultDeletedWithTimeout("pipelinerun", prName, lastNamespace, 210*time.Second) // 3.5m for 2m retention + CronJob delay
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
