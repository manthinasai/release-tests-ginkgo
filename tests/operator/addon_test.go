package operator_test

import (
	"log"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Verify Addon E2E", Serial, Ordered, ContinueOnFailure,
	Label("e2e", "integration", "operator", "addon", "admin"), func() {

		BeforeAll(func() {
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())

			DeferCleanup(func() {
				log.Println("Restoring addon config to defaults: resolverTasks=true, pipelineTemplates=true")
				cmd.MustSucceed("oc", "patch", "TektonConfig", "config", "--type=merge", "-p",
					`{"spec":{"addon":{"params":[{"name":"resolverTasks","value":"true"},{"name":"pipelineTemplates","value":"true"}]}}}`)
			})
		})

		// PIPELINES-15-TC06
		It("Disable/Enable resolverTasks", Label("sanity", "resolvertasks"), func() {
			oc.UpdateAddonConfig("false", "false", "")
			operator.AssertTaskPresence("openshift-pipelines", "s2i-java", false)

			oc.UpdateAddonConfig("true", "false", "")
			operator.AssertTaskPresence("openshift-pipelines", "s2i-java", true)
		})

		// PIPELINES-15-TC07
		It("Disable/Enable resolverTasks with additional Tasks", Label("resolvertasks"), func() {
			oc.UpdateAddonConfig("true", "false", "")
			operator.AssertTaskPresence("openshift-pipelines", "s2i-java", true)

			cmd.MustSucceed("oc", "apply", "-f", config.Path("testdata/ecosystem/tasks/hello.yaml"), "-n", "openshift-pipelines")
			DeferCleanup(func() {
				cmd.Run("oc", "delete", "task", "hello", "-n", "openshift-pipelines")
			})
			operator.AssertTaskPresence("openshift-pipelines", "hello", true)

			oc.UpdateAddonConfig("false", "false", "")
			operator.AssertTaskPresence("openshift-pipelines", "s2i-java", false)
			operator.AssertTaskPresence("openshift-pipelines", "hello", true)

			oc.UpdateAddonConfig("true", "false", "")
			operator.AssertTaskPresence("openshift-pipelines", "s2i-java", true)
			operator.AssertTaskPresence("openshift-pipelines", "hello", true)
		})

		// PIPELINES-15-TC08
		It("Disable/Enable pipeline templates", Label("sanity", "resolvertasks"), func() {
			oc.UpdateAddonConfig("true", "true", "")
			operator.AssertPipelinesPresence("openshift", true)

			oc.UpdateAddonConfig("true", "false", "")
			operator.AssertPipelinesPresence("openshift", false)

			oc.UpdateAddonConfig("true", "true", "")
			operator.AssertPipelinesPresence("openshift", true)
		})

		// PIPELINES-15-TC05
		It("Enable pipeline templates when clustertask is disabled", Label("negative"), func() {
			oc.UpdateAddonConfig("false", "true",
				"pipelineTemplates cannot be true if resolverTask is false")
		})

		// PIPELINES-15-TC09
		It("Verify versioned ecosystem tasks", func() {
			operator.VerifyVersionedTasks()
		})

		// PIPELINES-15-TC10
		It("Verify versioned stepaction tasks", func() {
			operator.VerifyVersionedStepActions()
		})
	})
