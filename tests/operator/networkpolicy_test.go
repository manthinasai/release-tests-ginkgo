package operator_test

import (
	"fmt"
	"log"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	approvalgate "github.com/openshift-pipelines/release-tests-ginkgo/pkg/manualapprovalgate"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// componentNPConfig maps a component to its expected NetworkPolicy names and operator CR type.
type componentNPConfig struct {
	policies    []string
	componentCR string // operator CR resource type (e.g. "tektonpipeline")
}

// networkPoliciesByComponent defines the expected NetworkPolicies per operator component.
var networkPoliciesByComponent = map[string]componentNPConfig{
	"pipelines": {
		componentCR: "tektonpipeline",
		policies: []string{
			"pipeline-default-deny",
			"pipeline-controller",
			"pipeline-webhook",
			"pipeline-events-controller",
			"pipeline-resolvers",
			"tekton-proxy-webhook-default-deny",
			"proxy-webhook",
		},
	},
	"triggers": {
		componentCR: "tektontrigger",
		policies: []string{
			"tekton-default-deny",
			"triggers-controller",
			"triggers-webhook",
			"triggers-core-interceptors",
		},
	},
	"chains": {
		componentCR: "tektonchain",
		policies: []string{
			"chains-controller-default-deny",
			"chains-controller",
		},
	},
	"results": {
		componentCR: "tektonresult",
		policies: []string{
			"results-default-deny",
			"results-api",
			"results-watcher",
			"results-retention-policy-agent",
			"results-postgres",
		},
	},
	"pruner": {
		componentCR: "tektonpruner",
		policies: []string{
			"tekton-pruner-default-deny",
			"pruner-controller",
			"pruner-webhook",
		},
	},
	"manual-approval-gate": {
		componentCR: "manualapprovalgate",
		policies: []string{
			"mag-default-deny",
			"mag-controller",
			"mag-webhook",
		},
	},
	"pac": {
		componentCR: "openshiftpipelinesascode",
		policies: []string{
			"pac-default-deny",
			"pac-controller",
			"pac-watcher",
			"pac-webhook",
		},
	},
}

// allNPComponentNames returns sorted component keys from the networkPoliciesByComponent map.
func allNPComponentNames() []string {
	names := make([]string, 0, len(networkPoliciesByComponent))
	for k := range networkPoliciesByComponent {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// assertNetworkPoliciesForComponent verifies all expected NetworkPolicies for a component.
func assertNetworkPoliciesForComponent(namespace, component string, shouldBePresent bool) {
	cfg, ok := networkPoliciesByComponent[component]
	Expect(ok).To(BeTrue(), "unknown component %q", component)

	result := cmd.Run("oc", "get", cfg.componentCR, "-o", "name")
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout()) == "" {
		Skip(fmt.Sprintf("Component %q not installed (no %s CR found)", component, cfg.componentCR))
	}

	for _, policyName := range cfg.policies {
		assertNetworkPolicyPresence(policyName, namespace, shouldBePresent)
	}
}

// assertNetworkPolicyPresence polls until the named NetworkPolicy is present or absent.
func assertNetworkPolicyPresence(name, namespace string, shouldBePresent bool) {
	if shouldBePresent {
		log.Printf("Waiting for NetworkPolicy %q to be present in namespace %q", name, namespace)
		Eventually(func(g Gomega) {
			result := cmd.Run("oc", "get", "networkpolicy", name, "-n", namespace, "-o", "name")
			g.Expect(result.ExitCode).To(Equal(0),
				"expected NetworkPolicy %s to be present in namespace %s", name, namespace)
			g.Expect(result.Stdout()).To(ContainSubstring(name))
		}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(Succeed())
	} else {
		log.Printf("Waiting for NetworkPolicy %q to be absent from namespace %q", name, namespace)
		Eventually(func(g Gomega) {
			result := cmd.Run("oc", "get", "networkpolicy", name, "-n", namespace, "-o", "name")
			g.Expect(result.Stderr()).To(ContainSubstring("not found"),
				"expected NotFound for NetworkPolicy %s in namespace %s, got stderr: %s",
				name, namespace, result.Stderr())
		}).WithTimeout(config.APITimeout).WithPolling(config.APIRetry).Should(Succeed())
	}
}

// setNetworkPolicyOnTektonConfig enables or disables NetworkPolicy on TektonConfig and MAG.
func setNetworkPolicyOnTektonConfig(disabled bool) {
	patchData := fmt.Sprintf(`{"spec":{"networkPolicy":{"disabled":%t}}}`, disabled)
	cmd.MustSucceed("oc", "patch", "TektonConfig", "config", "--type=merge", "-p", patchData)
	log.Printf("Patched TektonConfig networkPolicy.disabled=%t", disabled)

	// ManualApprovalGate has its own CR and must be patched separately.
	result := cmd.Run("oc", "get", "manualapprovalgate", "manual-approval-gate", "-o", "name")
	if result.ExitCode == 0 && strings.Contains(result.Stdout(), "manual-approval-gate") {
		cmd.MustSucceed("oc", "patch", "manualapprovalgate", "manual-approval-gate",
			"--type=merge", "-p", patchData)
		log.Printf("Patched ManualApprovalGate networkPolicy.disabled=%t", disabled)
	}
}

// ensureNetworkPolicyEnabled re-enables NetworkPolicy and waits for TektonConfig to stabilize.
func ensureNetworkPolicyEnabled() {
	setNetworkPolicyOnTektonConfig(false)
	operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())
}

var _ = Describe("Verify NetworkPolicy", Serial, Ordered, ContinueOnFailure,
	Label("e2e", "operator", "admin", "networkpolicy"), func() {
		BeforeEach(func() {
			lastNamespace = config.TargetNamespace
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())
		})

		// Suite-level setup: enable TektonPruner and deploy MAG if needed.
		BeforeAll(func() {
			enabledPruner := false
			deployedMAG := false

			// Capture original pruner state for restoration.
			origPrunerDisabled := operator.GetTektonConfigField("{.spec.pruner.disabled}")
			origTektonPrunerDisabled := operator.GetTektonConfigField("{.spec.tektonpruner.disabled}")

			// Enable TektonPruner if not already enabled (must disable old pruner — can't have both).
			if origTektonPrunerDisabled != "false" {
				log.Println("Enabling TektonPruner (disabling old pruner)")
				cmd.MustSucceed("oc", "patch", "TektonConfig", "config", "--type=merge", "-p",
					`{"spec":{"pruner":{"disabled":true},"tektonpruner":{"disabled":false}}}`)
				enabledPruner = true
			}

			// Deploy ManualApprovalGate if not already present.
			result := cmd.Run("oc", "get", "manualapprovalgate", "manual-approval-gate", "-o", "name")
			if result.ExitCode != 0 || !strings.Contains(result.Stdout(), "manual-approval-gate") {
				log.Println("Deploying ManualApprovalGate")
				cmd.MustSucceed("oc", "apply", "-f",
					config.Path("testdata/manualapprovalgate/manual-approval-gate.yaml"))
				deployedMAG = true
			}

			operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

			if deployedMAG {
				approvalgate.ValidateMAGDeployment(sharedClients)
			}

			DeferCleanup(func() {
				log.Println("Restoring NetworkPolicy on TektonConfig (enabled)")
				setNetworkPolicyOnTektonConfig(false)

				if enabledPruner {
					restorePruner := origPrunerDisabled
					if restorePruner == "" {
						restorePruner = "false"
					}
					restoreTektonPruner := origTektonPrunerDisabled
					if restoreTektonPruner == "" {
						restoreTektonPruner = "true"
					}
					log.Printf("Restoring pruner state: pruner.disabled=%s, tektonpruner.disabled=%s",
						restorePruner, restoreTektonPruner)
					cmd.Run("oc", "patch", "TektonConfig", "config", "--type=merge", "-p",
						fmt.Sprintf(`{"spec":{"pruner":{"disabled":%s},"tektonpruner":{"disabled":%s}}}`,
							restorePruner, restoreTektonPruner))
				}

				if deployedMAG {
					log.Println("Deleting ManualApprovalGate")
					cmd.Run("oc", "delete", "manualapprovalgate", "manual-approval-gate")
				}

				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())
			})
		})

		// TC01: Verify NetworkPolicies exist for all components
		Context("NetworkPolicies exist for all components: PIPELINES-38-TC01", Ordered, Label("sanity"), func() {
			It("should have NetworkPolicies present for every component", func() {
				for _, component := range allNPComponentNames() {
					assertNetworkPoliciesForComponent(config.TargetNamespace, component, true)
				}
			})
		})

		// TC02: Disable NetworkPolicy via TektonConfig
		Context("Disable NetworkPolicy via TektonConfig: PIPELINES-38-TC02", Ordered, func() {
			It("should remove all NetworkPolicies when disabled", func() {
				setNetworkPolicyOnTektonConfig(true)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				for _, component := range allNPComponentNames() {
					assertNetworkPoliciesForComponent(config.TargetNamespace, component, false)
				}

				DeferCleanup(ensureNetworkPolicyEnabled)
			})
		})

		// TC03: Re-enable NetworkPolicy via TektonConfig
		Context("Re-enable NetworkPolicy via TektonConfig: PIPELINES-38-TC03", Ordered, func() {
			It("should restore all NetworkPolicies when re-enabled", func() {
				// Disable first so re-enable is meaningful regardless of prior TC state.
				setNetworkPolicyOnTektonConfig(true)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				setNetworkPolicyOnTektonConfig(false)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				for _, component := range allNPComponentNames() {
					assertNetworkPoliciesForComponent(config.TargetNamespace, component, true)
				}
			})
		})

		// TC04: Add custom NetworkPolicy via TektonConfig
		Context("Add custom NetworkPolicy via TektonConfig: PIPELINES-38-TC04", Ordered, func() {
			It("should manage custom NetworkPolicy lifecycle", func() {
				ensureNetworkPolicyEnabled()

				const customPolicy = "custom-test-deny-all"

				By("Adding a custom NetworkPolicy")
				patchData := fmt.Sprintf(
					`{"spec":{"networkPolicy":{"policies":{"%s":{"podSelector":{},"policyTypes":["Ingress","Egress"]}}}}}`,
					customPolicy,
				)
				cmd.MustSucceed("oc", "patch", "TektonConfig", "config", "--type=merge", "-p", patchData)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				assertNetworkPolicyPresence(customPolicy, config.TargetNamespace, true)
				assertNetworkPoliciesForComponent(config.TargetNamespace, "pipelines", true)

				By("Removing the custom policy and disabling NetworkPolicy")
				cmd.MustSucceed("oc", "patch", "TektonConfig", "config", "--type=json",
					"-p", `[{"op":"remove","path":"/spec/networkPolicy/policies"}]`)
				setNetworkPolicyOnTektonConfig(true)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				assertNetworkPolicyPresence(customPolicy, config.TargetNamespace, false)

				By("Re-enabling NetworkPolicy and verifying custom policy stays removed")
				setNetworkPolicyOnTektonConfig(false)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				assertNetworkPolicyPresence(customPolicy, config.TargetNamespace, false)
				assertNetworkPoliciesForComponent(config.TargetNamespace, "pipelines", true)
			})
		})

		// TC05: Add custom NetworkPolicy on standalone component
		Context("Add custom NetworkPolicy on standalone component: PIPELINES-38-TC05", Ordered, func() {
			It("should manage custom NetworkPolicy on ManualApprovalGate", func() {
				ensureNetworkPolicyEnabled()

				const customPolicy = "mag-custom-test"

				result := cmd.Run("oc", "get", "manualapprovalgate", "manual-approval-gate", "-o", "name")
				if result.ExitCode != 0 || !strings.Contains(result.Stdout(), "manual-approval-gate") {
					Skip("ManualApprovalGate not found, skipping")
				}

				By("Adding a custom NetworkPolicy on ManualApprovalGate")
				patchData := fmt.Sprintf(
					`{"spec":{"networkPolicy":{"policies":{"%s":{"podSelector":{},"policyTypes":["Ingress","Egress"]}}}}}`,
					customPolicy,
				)
				cmd.MustSucceed("oc", "patch", "manualapprovalgate", "manual-approval-gate",
					"--type=merge", "-p", patchData)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				assertNetworkPolicyPresence(customPolicy, config.TargetNamespace, true)

				By("Removing the custom policy and disabling NetworkPolicy")
				cmd.MustSucceed("oc", "patch", "manualapprovalgate", "manual-approval-gate",
					"--type=json", "-p", `[{"op":"remove","path":"/spec/networkPolicy/policies"}]`)
				setNetworkPolicyOnTektonConfig(true)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				assertNetworkPolicyPresence(customPolicy, config.TargetNamespace, false)

				By("Re-enabling NetworkPolicy and verifying custom policy stays removed")
				setNetworkPolicyOnTektonConfig(false)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())

				assertNetworkPolicyPresence(customPolicy, config.TargetNamespace, false)
				assertNetworkPoliciesForComponent(config.TargetNamespace, "manual-approval-gate", true)
			})
		})
	})
