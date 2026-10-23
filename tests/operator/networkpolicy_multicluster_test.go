package operator_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/multicluster"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

const multiclusterCustomNP = "multicluster-custom-test"

var _ = Describe("Verify multicluster NetworkPolicy: PIPELINES-39", Serial, Ordered, ContinueOnFailure,
	Label("operator", "admin", "networkpolicy", "multicluster"), func() {
		var env *multicluster.Environment

		BeforeEach(func() {
			lastNamespace = config.TargetNamespace
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())
		})

		// Suite-level setup: configure Kueue and register the cluster as its own
		// MultiKueue worker. Teardown restores the exact pre-test TektonConfig
		// state and deletes every resource this run created.
		BeforeAll(func(specCtx SpecContext) {
			env = multicluster.NewEnvironment(sharedClients)
			DeferCleanup(func(cleanupCtx SpecContext) {
				Expect(env.Cleanup(cleanupCtx)).To(Succeed(), "failed to restore the cluster")
			})
			Expect(env.Setup(specCtx)).To(Succeed(), "failed to prepare the hub environment")
		})

		// TC07: Hub deploys the scheduler, proxy, and syncer policies.
		Context("Hub maintains every multicluster NetworkPolicy: PIPELINES-39-TC01", Ordered, func() {
			It("should create the scheduler, proxy, and syncer policies", func(specCtx SpecContext) {
				Expect(env.SetSchedulerMode(false, false, "Hub")).To(Succeed())
				Expect(env.AssertPolicies(specCtx, true, "scheduler", "proxy-aae", "syncer-service")).To(Succeed())
			})
		})

		// TC08: Hub to Spoke must tear the hub-only policies down.
		Context("Switching the hub to a spoke removes hub-only policies: PIPELINES-39-TC02", Ordered, func() {
			It("should keep the scheduler policies and remove the proxy and syncer policies", func(specCtx SpecContext) {
				Expect(env.SetSchedulerMode(false, false, "Spoke")).To(Succeed())
				Expect(env.AssertPolicies(specCtx, true, "scheduler")).To(Succeed())
				Expect(env.AssertPolicies(specCtx, false, multicluster.HubOnlyComponents...)).To(Succeed())
			})
		})

		// TC09: Spoke to Hub must restore them.
		Context("Switching back to a hub restores hub-only policies: PIPELINES-39-TC03", Ordered, func() {
			It("should recreate the proxy and syncer policies", func(specCtx SpecContext) {
				Expect(env.SetSchedulerMode(false, false, "Hub")).To(Succeed())
				Expect(env.AssertPolicies(specCtx, true, "scheduler", "proxy-aae", "syncer-service")).To(Succeed())
			})
		})

		// TC10: Disabling multi-cluster collapses the hub-only components.
		Context("Disabling multi-cluster removes hub-only policies: PIPELINES-39-TC04", Ordered, func() {
			It("should keep the scheduler policies and remove the proxy and syncer policies", func(specCtx SpecContext) {
				Expect(env.SetSchedulerMode(false, true, "")).To(Succeed())
				Expect(env.AssertPolicies(specCtx, true, "scheduler")).To(Succeed())
				Expect(env.AssertPolicies(specCtx, false, multicluster.HubOnlyComponents...)).To(Succeed())
			})
		})

		// TC11: networkPolicy.disabled wins over the scheduler mode.
		Context("Disabling NetworkPolicy removes every multicluster policy: PIPELINES-39-TC05", Ordered, func() {
			It("should remove all policies when disabled and restore them when re-enabled", func(specCtx SpecContext) {
				Expect(env.SetSchedulerMode(false, false, "Hub")).To(Succeed())

				By("Disabling NetworkPolicy on TektonConfig")
				setNetworkPolicyOnTektonConfig(true)
				operator.EnsureTektonConfigStatusInstalled(sharedClients.TektonConfig(), store.GetCRNames())
				Expect(env.AssertPolicies(specCtx, false, "scheduler", "proxy-aae", "syncer-service")).To(Succeed())

				By("Re-enabling NetworkPolicy on TektonConfig")
				ensureNetworkPolicyEnabled()
				Expect(env.AssertPolicies(specCtx, true, "scheduler", "proxy-aae", "syncer-service")).To(Succeed())

				DeferCleanup(ensureNetworkPolicyEnabled)
			})
		})

		// TC12: a custom policy merges with the multicluster defaults.
		//
		// The custom policy is declared on TektonConfig rather than on the proxy CR.
		// TektonConfig owns spec.networkPolicy for every component it creates and copies
		// its own value over the component's on each reconcile, so patching
		// TektonMulticlusterProxyAAE directly is not a supported way to configure it.
		Context("Custom NetworkPolicy alongside the multicluster defaults: PIPELINES-39-TC06", Ordered, func() {
			It("should add the custom policy alongside the defaults", func(specCtx SpecContext) {
				Expect(env.SetSchedulerMode(false, false, "Hub")).To(Succeed())

				origNP, err := env.EnsureCustomPolicy(specCtx, multiclusterCustomNP)
				Expect(err).NotTo(HaveOccurred(), "failed to add the custom policy")
				// Dropping the policies key does not delete the NetworkPolicy, so the custom
				// set has to be torn down with networkPolicy.disabled before the original
				// configuration is restored.
				DeferCleanup(func(cleanupCtx SpecContext) {
					Expect(env.RemoveCustomPolicyAndRestore(cleanupCtx, multiclusterCustomNP, origNP)).To(Succeed())
				})
			})
		})
	})
