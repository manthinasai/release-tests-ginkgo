package operator_test

import (
	"log"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Verify Addon E2E: PIPELINES-15", Serial, Ordered, ContinueOnFailure,
	Label("e2e", "integration", "operator", "addon", "admin"), func() {

		BeforeAll(func() {
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())

			DeferCleanup(func() {
				log.Println("Restoring addon config to defaults: resolverTasks=true, pipelineTemplates=true")
				cmd.MustSucceed("oc", "patch", "TektonConfig", "config", "--type=merge", "-p",
					`{"spec":{"addon":{"params":[{"name":"resolverTasks","value":"true"},{"name":"pipelineTemplates","value":"true"}]}}}`)
			})
		})

		It("Disable/Enable resolverTasks: PIPELINES-15-TC06", Label("sanity", "resolvertasks"), func() {
			oc.UpdateAddonConfig("false", "false", "")
			operator.AssertTaskPresence("openshift-pipelines", "s2i-java", false)

			oc.UpdateAddonConfig("true", "false", "")
			operator.AssertTaskPresence("openshift-pipelines", "s2i-java", true)
		})

		It("Disable/Enable resolverTasks with additional Tasks: PIPELINES-15-TC07", Label("resolvertasks"), func() {
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

		It("Disable/Enable pipeline templates: PIPELINES-15-TC08", Label("sanity", "resolvertasks"), func() {
			oc.UpdateAddonConfig("true", "true", "")
			operator.AssertPipelinesPresence("openshift", true)

			oc.UpdateAddonConfig("true", "false", "")
			operator.AssertPipelinesPresence("openshift", false)

			oc.UpdateAddonConfig("true", "true", "")
			operator.AssertPipelinesPresence("openshift", true)
		})

		It("Enable pipeline templates when clustertask is disabled: PIPELINES-15-TC05", Label("negative"), func() {
			oc.UpdateAddonConfig("false", "true",
				"pipelineTemplates cannot be true if resolverTask is false")
		})

		It("Verify versioned ecosystem tasks: PIPELINES-15-TC09", func() {
			operator.VerifyVersionedTasks()
		})

		It("Verify versioned stepaction tasks: PIPELINES-15-TC10", func() {
			operator.VerifyVersionedStepActions()
		})
	})
