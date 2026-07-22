package operator_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// Each inner Describe gets its own namespace via hooks.AutoNamespacePerDescribe
// (registered in suite_test.go). The outer Ordered+Serial container guarantees
// TC-01 through TC-11 execute sequentially, matching the Gauge spec order.
// Tekton-pruner is enabled by TC-01 and stays enabled through TC-11; the AfterAll
// restores the cluster to its pre-test state (legacy pruner re-enabled).
var _ = Describe("Verify Tekton Pruner Functionality", Serial, Ordered, ContinueOnFailure,
	Label("e2e", "integration", "operator", "admin", "tekton-pruner", "pruner"), func() {

		// Matches Gauge's spec-level Pre condition — runs once before all TCs.
		BeforeAll(func() {
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())
		})

		AfterAll(func() {
			oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "global", "")
			oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "null", "")
			oc.SetTektonPrunerGlobalConfig("successfulHistoryLimit", "null", "")
			oc.SetTektonPrunerGlobalConfig("failedHistoryLimit", "null", "")
			oc.DisableTektonPruner()
			oc.EnableLegacyPruner()
		})

		// PIPELINES-36-TC-01
		Describe("Enable Tekton Pruner & Validate Deployment Status", Label("sanity"), func() {
			It("should enable tekton-pruner and validate controller and webhook deployments", func() {
				oc.DisableLegacyPruner()
				oc.EnableTektonPruner()
				k8s.ValidateDeployments(sharedClients, config.TargetNamespace,
					"tekton-pruner-controller", "tekton-pruner-webhook")
				opc.AssertComponentVersion(os.Getenv("PRUNER_VERSION"), "pruner")
			})
		})

		// PIPELINES-36-TC-02
		Describe("Webhook: Negative Values & Invalid Type", func() {
			It("should reject invalid tekton-pruner global-config values via webhook", func() {
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "-1",
					"ttlSecondsAfterFinished cannot be negative")
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "60s",
					"cannot unmarshal string")
				oc.SetTektonPrunerGlobalConfig("successfulHistoryLimit", "not-a-number",
					"cannot unmarshal string")
			})
		})

		// PIPELINES-36-TC-03
		Describe("Global TTL Expiry for PipelineRuns", Label("sanity"), func() {
			It("should prune all pipelineruns after ttlSecondsAfterFinished expires", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "global", "")
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "60", "")
				oc.Create("testdata/pruner/pipeline/pipeline-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-for-pruner.yaml", ns)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 5, 60)
				// INTENTIONAL SLEEP: wait for ttlSecondsAfterFinished=60 to fire.
				time.Sleep(60 * time.Second)
				pipelines.AssertNumberOfPipelineruns(ns, 0, 30)
			})
		})

		// PIPELINES-36-TC-04
		Describe("Global TTL Expiry for TaskRuns", func() {
			It("should prune all taskruns after ttlSecondsAfterFinished expires", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "global", "")
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "60", "")
				oc.Create("testdata/pruner/task/task-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/task/taskrun-for-pruner.yaml", ns)
				pipelines.AssertNumberOfTaskrunsWithStatus(ns, "Succeeded", 5, 60)
				// INTENTIONAL SLEEP: wait for ttlSecondsAfterFinished=60 to fire.
				time.Sleep(60 * time.Second)
				pipelines.AssertNumberOfTaskruns(ns, 0, 30)
			})
		})

		// PIPELINES-36-TC-05
		Describe("Successful History Limit", Label("sanity"), func() {
			It("should keep only the N most recent successful pipelineruns", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "global", "")
				oc.Create("testdata/pruner/pipeline/pipeline-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-for-pruner.yaml", ns)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 5, 60)
				oc.SetTektonPrunerGlobalConfig("successfulHistoryLimit", "2", "")
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 2, 60)
			})
		})

		// PIPELINES-36-TC-06
		Describe("Failed History Limit", func() {
			It("should keep only the N most recent failed pipelineruns", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "global", "")
				oc.Create("testdata/pruner/pipeline/pipeline-fail-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-fail-for-pruner.yaml", ns)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Failed", 5, 60)
				oc.SetTektonPrunerGlobalConfig("failedHistoryLimit", "3", "")
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Failed", 3, 60)
			})
		})

		// PIPELINES-36-TC-07
		Describe("Mixed History Limits", Label("sanity"), func() {
			It("should enforce successfulHistoryLimit and failedHistoryLimit simultaneously", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("successfulHistoryLimit", "5", "")
				oc.SetTektonPrunerGlobalConfig("failedHistoryLimit", "5", "")
				oc.Create("testdata/pruner/pipeline/pipeline-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipeline-fail-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-fail-for-pruner.yaml", ns)
				pipelines.AssertNumberOfPipelineruns(ns, 10, 60)
				oc.SetTektonPrunerGlobalConfig("successfulHistoryLimit", "2", "")
				oc.SetTektonPrunerGlobalConfig("failedHistoryLimit", "3", "")
				pipelines.AssertNumberOfPipelineruns(ns, 5, 60)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 2, 60)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Failed", 3, 60)
			})
		})

		// PIPELINES-36-TC-08
		Describe("Namespace Config Override Error", func() {
			It("should reject a namespace TTL that exceeds the global TTL limit", func() {
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "60", "")
				oc.SetTektonPrunerGlobalConfig("namespaces.dev.ttlSecondsAfterFinished", "300",
					"ttlSecondsAfterFinished (300) cannot exceed global limit")
			})
		})

		// PIPELINES-36-TC-09
		Describe("Label Selector Match & Mismatch", Label("sanity"), func() {
			It("should prune label-matching pipelineruns and retain non-matching ones", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "namespace", "")
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "60", "")
				oc.Create("testdata/pruner/pipeline/pipeline-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/configmap/label-prune-ns-cm.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-label-for-pruner.yaml", ns)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 1, 30)
				// INTENTIONAL SLEEP: wait for the label-selector TTL (30s) to fire.
				time.Sleep(30 * time.Second)
				pipelines.AssertNumberOfPipelineruns(ns, 0, 15)
				// Non-matching pipelinerun (type:nightly) must not be pruned by the label rule.
				oc.Create("testdata/pruner/pipeline/pipelinerun-nightly-label-for-pruner.yaml", ns)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 1, 30)
				// INTENTIONAL SLEEP: confirm the non-matching pipelinerun is retained.
				time.Sleep(30 * time.Second)
				pipelines.AssertNumberOfPipelineruns(ns, 1, 15)
			})
		})

		// PIPELINES-36-TC-10
		Describe("Annotation Selector", Label("sanity"), func() {
			It("should prune pipelineruns that match the annotation selector", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "namespace", "")
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "60", "")
				oc.Create("testdata/pruner/pipeline/pipeline-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/configmap/annotation-prune-ns-cm.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-annotation-for-pruner.yaml", ns)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 1, 30)
				// INTENTIONAL SLEEP: wait for the annotation-selector TTL (30s) to fire.
				time.Sleep(30 * time.Second)
				pipelines.AssertNumberOfPipelineruns(ns, 0, 15)
			})
		})

		// PIPELINES-36-TC-11
		Describe("AND Logic (Label + Annotation)", func() {
			It("should prune pipelineruns matching both label AND annotation selectors", func() {
				ns := store.Namespace()
				oc.SetTektonPrunerGlobalConfig("enforcedConfigLevel", "namespace", "")
				oc.SetTektonPrunerGlobalConfig("ttlSecondsAfterFinished", "60", "")
				oc.Create("testdata/pruner/pipeline/pipeline-for-pruner.yaml", ns)
				oc.Create("testdata/pruner/configmap/label-and-annotation-ns-cm.yaml", ns)
				oc.Create("testdata/pruner/pipeline/pipelinerun-label-annotation.yaml", ns)
				pipelines.AssertNumberOfPipelinerunsWithStatus(ns, "Succeeded", 1, 30)
				// INTENTIONAL SLEEP: wait for the AND-logic selector TTL (30s) to fire.
				time.Sleep(30 * time.Second)
				pipelines.AssertNumberOfPipelineruns(ns, 0, 15)
			})
		})
	})
