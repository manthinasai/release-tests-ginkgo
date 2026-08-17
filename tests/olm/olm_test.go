package olm_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega
	"github.com/tektoncd/operator/test/utils"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	olmpkg "github.com/openshift-pipelines/release-tests-ginkgo/pkg/olm"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
)

var oc = occmd.OC{}
var rnames = utils.ResourceNames{TektonConfig: "config"}

var _ = Describe("OLM Operator Lifecycle: PIPELINES-09", Serial, Label("olm", "admin"), func() {

	Describe("Install openshift-pipelines operator: PIPELINES-09-TC01", Label("install", "sanity"), Ordered, ContinueOnFailure, func() {
		It("subscribes to operator", func() {
			_, err := olmpkg.SubscribeAndWaitForOperatorToBeReady(
				sharedClients,
				config.Flags.SubscriptionName,
				config.Flags.Channel,
				config.Flags.CatalogSource,
			)
			Expect(err).NotTo(HaveOccurred(), "Failed to subscribe to operator")
		})

		It("waits for TektonConfig CR availability", func() {
			operator.WaitForTektonConfigCR(sharedClients, rnames)
		})

		// On a brand-new CI cluster the operator and pipelines webhook pods may not
		// be serving yet even though the CSV reports Succeeded. Any subsequent step
		// that patches a Tekton CR (or creates a Pipeline/TaskRun) hits the
		// admission webhook and fails with "connection refused" if we don't wait.
		It("waits for operator and pipelines webhook deployments to be ready", func() {
			operator.WaitForOperatorWebhooks(sharedClients)
		})

		It("defines artifact-hub-api variable", func() {
			operator.DefineArtifactHubAPIVariable()
		})

		It("verifies openshift-pipelines namespace exists", func() {
			operator.VerifyNamespaceExists("openshift-pipelines")
		})

		It("applies TektonHub resource", func() {
			oc.Apply("testdata/hub/tektonhub.yaml", "")
		})

		It("configures GitHub token for git resolver in TektonConfig", func() {
			operator.ConfigureGitResolverToken(sharedClients)
		})

		It("configures the bundles resolver", func() {
			operator.ConfigureBundlesResolver(sharedClients)
		})

		It("enables console plugin", func() {
			operator.EnableConsolePluginOperator(sharedClients)
		})

		It("enables statefulset in tektonconfig", func() {
			operator.EnableStatefulSet(sharedClients)
		})

		It("validates triggers deployment", func() {
			operator.ValidateTriggersDeployment(sharedClients)
		})

		It("validates PAC deployment", func() {
			operator.ValidatePACDeployment(sharedClients)
		})

		It("enables generateSigningSecret for Tekton Chains", func() {
			operator.EnableChainsSigningSecret(sharedClients)
		})

		It("validates hub deployment", func() {
			operator.ValidateHubDeployment(sharedClients)
		})

		It("enables statefulset for chains", func() {
			operator.EnableStatefulSetForComponent(sharedClients, "chains")
		})

		It("enables statefulset for results", func() {
			operator.EnableStatefulSetForComponent(sharedClients, "results")
		})

		It("validates tekton-pipelines-controller statefulset", func() {
			operator.ValidateStatefulSetDeployment(sharedClients, "tekton-pipelines-controller")
		})

		It("validates tekton-pipelines-remote-resolvers statefulset", func() {
			operator.ValidateStatefulSetDeployment(sharedClients, "tekton-pipelines-remote-resolvers")
		})

		It("validates tekton-chains-controller statefulset", func() {
			operator.ValidateStatefulSetDeployment(sharedClients, "tekton-chains-controller")
		})

		It("validates tekton-results-watcher statefulset", func() {
			operator.ValidateStatefulSetDeployment(sharedClients, "tekton-results-watcher")
		})

		It("validates tkn server cli deployment", func() {
			operator.ValidateTknServerCLI(sharedClients)
		})

		It("validates console plugin deployment", func() {
			operator.ValidateConsolePluginDeployment(sharedClients)
		})

		It("configures Results with Loki", func() {
			operator.ConfigureResultsWithLoki(sharedClients)
		})

		It("creates Results route", func() {
			operator.CreateResultsRoute()
		})

		It("ensures Tekton Results is ready", func() {
			operator.EnsureResultsReady()
		})

		It("verifies TektonAddons install status", func() {
			operator.VerifyTektonAddonsStatus(sharedClients)
		})

		It("validates RBAC", func() {
			operator.ValidateRBAC(sharedClients, rnames)
		})

		It("validates quickstarts", func() {
			opc.ValidateQuickstarts()
		})

		It("validates default auto prune cronjob", func() {
			operator.ValidateAutoPruneCronjob(sharedClients)
		})

		It("applies MAG resource", func() {
			oc.Apply("testdata/manualapprovalgate/manual-approval-gate.yaml", "")
		})

		It("validates MAG deployment", func() {
			operator.ValidateMAGDeployment(sharedClients)
		})

		It("validates tektoninstallersets status", func() {
			operator.ValidateTektonInstallerSetsStatus(sharedClients)
		})

		It("validates tektoninstallersets names", func() {
			operator.ValidateTektonInstallerSetsNames(sharedClients)
		})
	})

	Describe("Upgrade openshift-pipelines operator: PIPELINES-09-TC02", Label("upgrade"), Ordered, ContinueOnFailure, func() {
		It("upgrades operator subscription", func() {
			upgradeChannel := os.Getenv("UPGRADE_CHANNEL")
			if upgradeChannel == "" {
				Skip("UPGRADE_CHANNEL not set -- skipping upgrade test")
			}

			_, err := olmpkg.UptadeSubscriptionAndWaitForOperatorToBeReady(
				sharedClients,
				config.Flags.SubscriptionName,
				upgradeChannel,
			)
			Expect(err).NotTo(HaveOccurred(), "Failed to upgrade operator subscription")
		})

		It("waits for TektonConfig CR availability", func() {
			operator.WaitForTektonConfigCR(sharedClients, rnames)
		})

		It("validates operator is installed after upgrade", func() {
			operator.ValidateOperatorInstalled(sharedClients)
		})

		It("validates RBAC after upgrade", func() {
			operator.ValidateRBAC(sharedClients, rnames)
		})

		It("validates quickstarts after upgrade", func() {
			opc.ValidateQuickstarts()
		})
	})

	Describe("Uninstall openshift-pipelines operator: PIPELINES-09-TC03", Label("uninstall"), Ordered, ContinueOnFailure, func() {
		It("uninstalls the operator", func() {
			err := olmpkg.OperatorCleanup(sharedClients, config.Flags.SubscriptionName)
			Expect(err).NotTo(HaveOccurred(), "Failed to uninstall operator")
		})
	})
})
