package operator_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/openshift"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/triggers"
)

// PIPELINES-18: Pre-upgrade test suite
// Namespaces created here must persist for post-upgrade tests to verify.
var _ = Describe("Openshift Pipelines pre upgrade specs", Serial, Ordered, ContinueOnFailure,
	Label("operator", "admin", "pre-upgrade", "no-auto-namespace"), func() {

		BeforeAll(func() {
			// NOTE: Do NOT delete the pre-upgrade projects in DeferCleanup.
			// They must persist for post-upgrade tests to verify.
		})

		// PIPELINES-18-TC01
		It("Setup environment for upgrade test", func() {
			ns := "releasetest-upgrade-triggers"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			oc.CreateNewNamespace(ns)

			// Verify pipeline SA exists
			operator.AssertServiceAccountPresent(sharedClients, ns, "pipeline")

			// Create trigger resources: embedded trigger template and eventlistener
			oc.Create("testdata/triggers/github-ctb/Embeddedtriggertemplate-git-push.yaml", ns)
			oc.Create("testdata/triggers/github-ctb/eventlistener-ctb-git-push.yaml", ns)

			// Verify imagestream "golang" exists
			tags := openshift.GetImageStreamTags(sharedClients, "openshift", "golang")
			Expect(tags).NotTo(BeEmpty(), "imagestream 'golang' should exist in openshift namespace")

			// Create & link github-secret
			oc.CreateSecretWithSecretToken("github-secret", ns)
			oc.LinkSecretToSA("github-secret", "pipeline", ns)

			// Expose eventlistener and send mock event
			route := triggers.ExposeEventListener(sharedClients, "listener-ctb-github-push", ns)
			resp, payload := triggers.MockPostEvent(route, "github", "push",
				"testdata/triggers/github-ctb/push.json", false)
			store.PutScenarioData("payload", string(payload))
			triggers.AssertElResponse(sharedClients, resp, "listener-ctb-github-push", ns)

			// Verify pipelinerun
			pipelines.ValidatePipelineRun(sharedClients, "pipelinerun-git-push-ctb", "successful", ns)
			oc.DeleteResourceInNamespace("pipelinerun", "pipelinerun-git-push-ctb", ns)

			// Create triggersCRD resources
			oc.Create("testdata/triggers/triggersCRD/eventlistener-triggerref.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/trigger.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/triggerbindings.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/triggersCRD/pipeline.yaml", ns)

			// Expose eventlistener and send mock PR event
			route2 := triggers.ExposeEventListener(sharedClients, "listener-triggerref", ns)
			resp2, payload2 := triggers.MockPostEvent(route2, "github", "pull_request",
				"testdata/triggers/triggersCRD/pull-request.json", false)
			store.PutScenarioData("payload", string(payload2))
			triggers.AssertElResponse(sharedClients, resp2, "listener-triggerref", ns)

			pipelines.ValidatePipelineRun(sharedClients, "parallel-pipelinerun", "successful", ns)
			oc.DeleteResourceInNamespace("pipelinerun", "parallel-pipelinerun", ns)

			// Create bitbucket resources
			oc.Create("testdata/triggers/bitbucket/bitbucket-eventlistener-interceptor.yaml", ns)
			oc.CreateSecretWithSecretToken("bitbucket-secret", ns)
			oc.LinkSecretToSA("bitbucket-secret", "pipeline", ns)

			route3 := triggers.ExposeEventListener(sharedClients, "bitbucket-listener", ns)
			resp3, payload3 := triggers.MockPostEvent(route3, "bitbucket", "refs_changed",
				"testdata/triggers/bitbucket/refs-change-event.json", false)
			store.PutScenarioData("payload", string(payload3))
			triggers.AssertElResponse(sharedClients, resp3, "bitbucket-listener", ns)

			pipelines.ValidateTaskRun(sharedClients, "bitbucket-run", "Failure", ns)
			oc.DeleteResourceInNamespace("taskrun", "bitbucket-run", ns)
		})

		// PIPELINES-18-TC03
		It("Setup Eventlistener with TLS enabled pre upgrade", Label("e2e", "sanity", "tls", "triggers"), func() {
			ns := "releasetest-upgrade-tls"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			oc.CreateNewNamespace(ns)

			oc.EnableTLSConfigForEventlisteners(ns)

			oc.Create("testdata/triggers/sample-pipeline.yaml", ns)
			oc.Create("testdata/triggers/triggerbindings/triggerbinding.yaml", ns)
			oc.Create("testdata/triggers/triggertemplate/triggertemplate.yaml", ns)
			oc.Create("testdata/triggers/eventlisteners/eventlistener-embeded-binding.yaml", ns)

			route := triggers.ExposeEventListenerForTLS(sharedClients, "listener-embed-binding", ns)
			resp, payload := triggers.MockPostEvent(route, "github", "push",
				"testdata/push.json", true)
			store.PutScenarioData("payload", string(payload))
			triggers.AssertElResponse(sharedClients, resp, "listener-embed-binding", ns)

			pipelines.ValidatePipelineRun(sharedClients, "simple-pipeline-run", "successful", ns)
			oc.DeleteResourceInNamespace("pipelinerun", "simple-pipeline-run", ns)
		})

		// PIPELINES-18-TC04
		It("Setup link secret to pipeline SA", Label("e2e", "sanity", "non-admin", "clustertasks", "git-clone"), func() {
			ns := "releasetest-upgrade-pipelines"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			oc.CreateNewNamespace(ns)

			operator.AssertServiceAccountPresent(sharedClients, ns, "pipeline")

			oc.Create("testdata/ecosystem/pipelines/git-clone-read-private.yaml", ns)
			oc.Create("testdata/pvc/pvc.yaml", ns)
			oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)
			oc.LinkSecretToSA("ssh-key", "pipeline", ns)

			oc.Create("testdata/ecosystem/pipelineruns/git-clone-read-private.yaml", ns)
			pipelines.ValidatePipelineRun(sharedClients, "git-clone-read-private-pipeline-run", "successful", ns)
			oc.DeleteResourceInNamespace("pipelinerun", "git-clone-read-private-pipeline-run", ns)
		})

		// PIPELINES-18-TC05
		It("Setup S2I golang pipeline pre upgrade", Label("e2e", "non-admin", "clustertasks", "s2i"), func() {
			ns := "releasetest-upgrade-s2i"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			oc.CreateNewNamespace(ns)

			operator.AssertServiceAccountPresent(sharedClients, ns, "pipeline")

			oc.Create("testdata/ecosystem/pipelines/s2i-go.yaml", ns)
			oc.Create("testdata/pvc/pvc.yaml", ns)
		})
	})
