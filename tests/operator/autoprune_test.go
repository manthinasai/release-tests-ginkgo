package operator_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Verify auto-prune E2E: PIPELINES-12", Serial,
	Label("e2e", "integration", "operator", "auto-prune", "admin"), func() {

		BeforeEach(func() {
			lastNamespace = store.Namespace()
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())
		})

		Context("Verify auto prune for taskrun: PIPELINES-12-TC01", Ordered, ContinueOnFailure, Label("sanity"), func() {
			It("should prune taskruns to keep 2", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.UpdatePrunerConfig("2", "*/1 * * * *", "taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("taskrun", 2, 180)
				operator.AssertResourceCount("pipelinerun", 5, 120)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune for pipelinerun: PIPELINES-12-TC02", Ordered, ContinueOnFailure, func() {
			It("should prune pipelineruns to keep 2", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.UpdatePrunerConfig("2", "*/1 * * * *", "pipelinerun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("pipelinerun", 2, 120)
				operator.AssertResourceCount("taskrun", 7, 180)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune for pipelinerun and taskrun: PIPELINES-12-TC03", Ordered, ContinueOnFailure, func() {
			It("should prune both pipelineruns and taskruns to keep 2", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.UpdatePrunerConfig("2", "*/1 * * * *", "pipelinerun,taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("pipelinerun", 2, 120)
				operator.AssertResourceCount("taskrun", 2, 180)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune with keep-since: PIPELINES-12-TC04", Ordered, ContinueOnFailure, Label("sanity"), func() {
			It("should prune with keep-since strategy", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				// INTENTIONAL SLEEP: Creates a time gap between "old" and "new" resources
				// so that keep-since pruning can distinguish them. This is NOT a polling
				// wait -- it is required to establish resource age difference.
				time.Sleep(120 * time.Second)

				operator.CreateAdditionalPrunerResources()

				oc.UpdatePrunerConfig("", "*/1 * * * *", "pipelinerun,taskrun", "2", false, true)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("pipelinerun", 5, 120)
				operator.AssertResourceCount("taskrun", 10, 180)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune skip namespace with annotation: PIPELINES-12-TC05", Ordered, ContinueOnFailure, func() {
			It("should not prune resources in namespace with prune.skip annotation", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.skip=true")

				oc.UpdatePrunerConfig("2", "*/1 * * * *", "pipelinerun,taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				// With skip annotation, resources should NOT be pruned
				operator.AssertResourceCount("pipelinerun", 5, 120)
				operator.AssertResourceCount("taskrun", 10, 180)

				// Remove skip annotation -- pruning should resume
				cmd.MustSucceed("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.skip-")
				operator.AssertResourceCount("pipelinerun", 2, 120)
				operator.AssertResourceCount("taskrun", 2, 180)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune add resources taskrun per namespace: PIPELINES-12-TC06", Ordered, ContinueOnFailure, func() {
			It("should only prune taskruns when namespace annotation specifies taskrun", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.resources=taskrun")

				oc.UpdatePrunerConfig("2", "*/1 * * * *", "pipelinerun,taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				// With resources=taskrun annotation, only taskruns should be pruned
				operator.AssertResourceCount("pipelinerun", 5, 120)
				operator.AssertResourceCount("taskrun", 2, 180)

				// Remove annotation -- pipelineruns should now also be pruned
				cmd.MustSucceed("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.resources-")
				operator.AssertResourceCount("pipelinerun", 2, 120)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune add resources taskrun and pipelinerun per namespace: PIPELINES-12-TC07", Ordered, ContinueOnFailure, Label("sanity"), func() {
			It("should prune both resources with per-namespace annotation override", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.resources=pipelinerun,taskrun")

				// Global config only prunes taskrun, but namespace annotation overrides to both
				oc.UpdatePrunerConfig("2", "*/1 * * * *", "taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("pipelinerun", 2, 120)
				operator.AssertResourceCount("taskrun", 2, 180)

				// Remove annotation and create more resources -- only taskrun should be pruned now
				cmd.MustSucceed("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.resources-")
				operator.CreateAdditionalPrunerResources()

				operator.AssertResourceCount("pipelinerun", 7, 120)
				operator.AssertResourceCount("taskrun", 2, 180)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune add keep per namespace with global keep: PIPELINES-12-TC08", Ordered, ContinueOnFailure, func() {
			It("should use per-namespace keep annotation override", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.keep=3")

				oc.UpdatePrunerConfig("2", "*/1 * * * *", "pipelinerun,taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				// Namespace annotation says keep=3
				operator.AssertResourceCount("pipelinerun", 3, 120)
				operator.AssertResourceCount("taskrun", 3, 180)

				// Remove annotation -- global keep=2 should apply
				cmd.MustSucceed("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.keep-")
				operator.AssertResourceCount("pipelinerun", 2, 120)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune with keep-since per namespace: PIPELINES-12-TC09", Ordered, ContinueOnFailure, func() {
			It("should use per-namespace keep-since annotation override", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				// INTENTIONAL SLEEP: Creates a time gap between "old" and "new" resources
				// for keep-since strategy differentiation. Not a polling scenario.
				time.Sleep(120 * time.Second)

				operator.CreateAdditionalPrunerResources()

				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.keep-since=2")

				// Global keep-since=10 (keeps more), namespace annotation keep-since=2 (keeps fewer)
				oc.UpdatePrunerConfig("", "*/1 * * * *", "pipelinerun,taskrun", "10", false, true)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("pipelinerun", 5, 120)
				operator.AssertResourceCount("taskrun", 10, 180)

				// Remove annotation
				cmd.MustSucceed("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.keep-since-")

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune with keep per namespace with global keep-since: PIPELINES-12-TC10", Ordered, ContinueOnFailure, func() {
			It("should use per-namespace keep with strategy annotation when global uses keep-since", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.keep=2")

				// Global uses keep-since=10. Namespace annotation uses keep=2 but
				// without strategy annotation, the global strategy is used.
				oc.UpdatePrunerConfig("", "*/1 * * * *", "pipelinerun,taskrun", "10", false, true)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("pipelinerun", 5, 120)
				operator.AssertResourceCount("taskrun", 10, 180)

				// Add strategy annotation -- now keep=2 should be applied
				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.strategy=keep")
				operator.AssertResourceCount("pipelinerun", 2, 120)
				operator.AssertResourceCount("taskrun", 2, 180)

				DeferCleanup(func() {
					cmd.Run("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.keep-")
					cmd.Run("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.strategy-")
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune schedule per namespace: PIPELINES-12-TC11", Ordered, ContinueOnFailure, func() {
			It("should use per-namespace schedule annotation override", func() {
				oc.RemovePrunerConfig()
				operator.CreatePrunerResources()

				// Namespace gets faster schedule via annotation
				oc.AnnotateNamespace(store.Namespace(), "operator.tekton.dev/prune.schedule=*/1 * * * *")

				// Global schedule is slow (*/8), but namespace annotation overrides to */1
				oc.UpdatePrunerConfig("2", "*/8 * * * *", "pipelinerun,taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				operator.AssertResourceCount("pipelinerun", 2, 120)
				operator.AssertResourceCount("taskrun", 2, 180)

				// Remove schedule annotation -- global slow schedule applies
				cmd.MustSucceed("oc", "annotate", "namespace", store.Namespace(), "operator.tekton.dev/prune.schedule-")

				// Wait for schedule change to take effect, then create more resources
				// The global schedule is */8 so pruning won't happen quickly
				Eventually(func(g Gomega) {
					// Verify the annotation was removed
					result := cmd.MustSucceed("oc", "get", "namespace", store.Namespace(),
						"-o", "jsonpath={.metadata.annotations.operator\\.tekton\\.dev/prune\\.schedule}")
					g.Expect(strings.TrimSpace(result.Stdout())).To(BeEmpty())
				}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

				operator.CreateAdditionalPrunerResources()

				// With slow global schedule, resources should accumulate
				operator.AssertResourceCount("pipelinerun", 7, 120)
				operator.AssertResourceCount("taskrun", 12, 180)

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
					operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)
				})
			})
		})

		Context("Verify auto prune validation: PIPELINES-12-TC12", Ordered, ContinueOnFailure, func() {
			It("should reject invalid pruner configurations", func() {
				oc.RemovePrunerConfig()

				// Test 1: Both keep and keep-since specified
				stderr1 := oc.UpdatePrunerConfigExpectError("2", "*/8 * * * *", "pipelinerun,taskrun", "2", true, true)
				Expect(stderr1).To(ContainSubstring("expected exactly one, got both: spec.pruner.keep, spec.pruner.keep-since"),
					"expected validation error for both keep and keep-since")

				// Test 2: Invalid resource type "taskrunas"
				stderr2 := oc.UpdatePrunerConfigExpectError("2", "*/8 * * * *", "pipelinerun,taskrunas", "", true, false)
				Expect(stderr2).To(ContainSubstring("invalid value: taskrunas"),
					"expected validation error for invalid resource taskrunas")

				// Test 3: Invalid resource type "pipelinerunas"
				stderr3 := oc.UpdatePrunerConfigExpectError("2", "*/8 * * * *", "pipelinerunas,taskrun", "", true, false)
				Expect(stderr3).To(ContainSubstring("invalid value: pipelinerunas"),
					"expected validation error for invalid resource pipelinerunas")
			})
		})

		Context("Verify auto prune cronjob stability: PIPELINES-12-TC13", Ordered, ContinueOnFailure, func() {
			It("should not re-create cronjob for random annotation/label changes", func() {
				oc.RemovePrunerConfig()

				oc.UpdatePrunerConfig("2", "10 * * * *", "pipelinerun,taskrun", "", true, false)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				preAnnotationName := operator.GetCronjobName(config.TargetNamespace, "10 * * * *")

				// Add random annotation
				oc.AnnotateNamespace(store.Namespace(), "random-annotation=true")
				// Brief wait for annotation propagation; not a polling scenario
				time.Sleep(5 * time.Second)

				postAnnotationName := operator.GetCronjobName(config.TargetNamespace, "10 * * * *")
				Expect(postAnnotationName).To(Equal(preAnnotationName),
					"cronjob should not be re-created after adding random annotation")

				// Remove random annotation
				cmd.MustSucceed("oc", "annotate", "namespace", store.Namespace(), "random-annotation-")
				postAnnotationRemovalName := operator.GetCronjobName(config.TargetNamespace, "10 * * * *")
				Expect(postAnnotationRemovalName).To(Equal(preAnnotationName),
					"cronjob should not be re-created after removing random annotation")

				// Add random label
				oc.LabelNamespace(store.Namespace(), "random=true")
				postLabelName := operator.GetCronjobName(config.TargetNamespace, "10 * * * *")
				Expect(postLabelName).To(Equal(preAnnotationName),
					"cronjob should not be re-created after adding random label")

				// Remove random label
				cmd.MustSucceed("oc", "label", "namespace", store.Namespace(), "random-")
				postLabelRemovalName := operator.GetCronjobName(config.TargetNamespace, "10 * * * *")
				Expect(postLabelRemovalName).To(Equal(preAnnotationName),
					"cronjob should not be re-created after removing random label")

				DeferCleanup(func() {
					oc.RemovePrunerConfig()
				})
			})
		})

		Context("Verify auto prune cronjob single container: PIPELINES-12-TC14", Ordered, ContinueOnFailure, Label("sanity", "cronjob"), func() {
			It("should have a single container in pruner cronjob", func() {
				oc.UpdatePrunerConfig("2", "20 * * * *", "taskrun", "", true, false)

				oc.CreateNewNamespace("test-project-1")
				oc.CreateNewNamespace("test-project-2")

				DeferCleanup(func() {
					oc.DeleteProjectIgnoreErrors("test-project-1")
					oc.DeleteProjectIgnoreErrors("test-project-2")
					oc.RemovePrunerConfig()
				})

				// Wait for cronjob to appear, then check container count
				Eventually(func(g Gomega) {
					output := cmd.MustSucceed("oc", "get", "cronjob", "-n", config.TargetNamespace,
						"-o", "jsonpath={range .items[*]}{.spec.jobTemplate.spec.template.spec.containers[*].name}{' '}{end}").Stdout()
					containers := strings.Fields(strings.TrimSpace(output))
					g.Expect(containers).To(HaveLen(1),
						"expected pruner cronjob to have 1 container, got %d", len(containers))
				}).WithTimeout(2 * time.Minute).WithPolling(config.APIRetry).Should(Succeed())
			})
		})

		Context("Verify operator stability after namespace deletion: PIPELINES-12-TC15", Ordered, ContinueOnFailure, Label("sanity", "cronjob"), func() {
			It("should remain stable after deleting namespace with pruner annotation", func() {
				oc.RemovePrunerConfig()
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, false)

				// Create namespace-one (has pruner annotations)
				ns := store.Namespace()
				oc.Create("testdata/pruner/namespaces/namespace-one.yaml", ns)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				// Delete namespace-one
				oc.DeleteProjectIgnoreErrors("namespace-one")
				// Wait for cronjob to disappear after namespace deletion
				Eventually(func(g Gomega) {
					output := cmd.Run("oc", "get", "cronjob", "-n", config.TargetNamespace, "-o", "name").Stdout()
					g.Expect(output).NotTo(ContainSubstring(config.PrunerNamePrefix),
						"expected cronjob to be removed after namespace deletion")
				}).WithTimeout(2 * time.Minute).WithPolling(config.APIRetry).Should(Succeed())

				// Verify operator is still installed
				operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())

				// Create namespace-two (also has pruner annotations)
				oc.Create("testdata/pruner/namespaces/namespace-two.yaml", ns)
				operator.AssertCronjobPresence(config.TargetNamespace, config.PrunerNamePrefix, true)

				// Delete namespace-two
				oc.DeleteProjectIgnoreErrors("namespace-two")
				Eventually(func(g Gomega) {
					output := cmd.Run("oc", "get", "cronjob", "-n", config.TargetNamespace, "-o", "name").Stdout()
					g.Expect(output).NotTo(ContainSubstring(config.PrunerNamePrefix),
						"expected cronjob to be removed after namespace deletion")
				}).WithTimeout(2 * time.Minute).WithPolling(config.APIRetry).Should(Succeed())
			})
		})
	})
