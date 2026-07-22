package operator_test

import (
	"log"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/openshift"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/triggers"
)

// PIPELINES-19: Post-upgrade test suite
// Verifies resources created in pre-upgrade tests survive the operator upgrade.
var _ = Describe("Olm Openshift Pipelines operator post upgrade tests", Serial, Ordered, ContinueOnFailure,
	Label("operator", "admin", "post-upgrade", "no-auto-namespace"), func() {

		BeforeAll(func() {
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())

			// Cleanup all upgrade projects after post-upgrade tests
			DeferCleanup(func() {
				log.Println("Cleaning up upgrade test namespaces")
				oc.DeleteProjectIgnoreErrors("releasetest-upgrade-triggers")
				oc.DeleteProjectIgnoreErrors("releasetest-upgrade-tls")
				oc.DeleteProjectIgnoreErrors("releasetest-upgrade-pipelines")
				oc.DeleteProjectIgnoreErrors("releasetest-upgrade-s2i")
			})
		})

		// PIPELINES-19-TC01
		It("Verify environment after upgrade", func() {
			ns := "releasetest-upgrade-triggers"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			cmd.MustSucceed("oc", "project", ns)

			// GitHub push event
			route1 := triggers.GetRoute("listener-ctb-github-push", ns)
			resp1, payload1 := triggers.MockPostEvent(route1, "github", "push",
				"testdata/triggers/github-ctb/push.json", false)
			store.PutScenarioData("payload", string(payload1))
			triggers.AssertElResponse(sharedClients, resp1, "listener-ctb-github-push", ns)
			pipelines.ValidatePipelineRun(sharedClients, "pipelinerun-git-push-ctb", "successful", ns)

			// GitHub PR event via triggersCRD
			route2 := triggers.GetRoute("listener-triggerref", ns)
			resp2, payload2 := triggers.MockPostEvent(route2, "github", "pull_request",
				"testdata/triggers/triggersCRD/pull-request.json", false)
			store.PutScenarioData("payload", string(payload2))
			triggers.AssertElResponse(sharedClients, resp2, "listener-triggerref", ns)
			pipelines.ValidatePipelineRun(sharedClients, "parallel-pipelinerun", "successful", ns)

			// Bitbucket event
			route3 := triggers.GetRoute("bitbucket-listener", ns)
			resp3, payload3 := triggers.MockPostEvent(route3, "bitbucket", "refs_changed",
				"testdata/triggers/bitbucket/refs-change-event.json", false)
			store.PutScenarioData("payload", string(payload3))
			triggers.AssertElResponse(sharedClients, resp3, "bitbucket-listener", ns)
			pipelines.ValidateTaskRun(sharedClients, "bitbucket-run", "Failure", ns)
		})

		// PIPELINES-19-TC03
		It("Verify Event listener with TLS after upgrade", Label("e2e", "sanity", "tls", "triggers"), func() {
			ns := "releasetest-upgrade-tls"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			cmd.MustSucceed("oc", "project", ns)

			route := triggers.GetRoute("listener-embed-binding", ns)
			resp, payload := triggers.MockPostEvent(route, "github", "push",
				"testdata/push.json", true)
			store.PutScenarioData("payload", string(payload))
			triggers.AssertElResponse(sharedClients, resp, "listener-embed-binding", ns)
			pipelines.ValidatePipelineRun(sharedClients, "simple-pipeline-run", "successful", ns)
		})

		// PIPELINES-19-TC04
		It("Verify secret is linked to SA even after upgrade", Label("e2e", "sanity", "non-admin", "clustertasks", "git-clone"), func() {
			ns := "releasetest-upgrade-pipelines"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			cmd.MustSucceed("oc", "project", ns)

			operator.AssertServiceAccountPresent(sharedClients, ns, "pipeline")

			oc.Create("testdata/ecosystem/pipelineruns/git-clone-read-private.yaml", ns)
			pipelines.ValidatePipelineRun(sharedClients, "git-clone-read-private-pipeline-run", "successful", ns)
		})

		// PIPELINES-19-TC05
		It("Verify S2I golang pipeline after upgrade", Label("e2e", "non-admin", "clustertasks", "s2i"), func() {
			ns := "releasetest-upgrade-s2i"
			lastNamespace = ns
			sharedClients.NewClientSet(ns)
			cmd.MustSucceed("oc", "project", ns)

			tags := openshift.GetImageStreamTags(sharedClients, "openshift", "golang")
			Expect(tags).NotTo(BeEmpty(), "golang imagestream tags should not be empty")

			for _, tag := range tags {
				if tag == "latest" {
					continue
				}
				log.Printf("Starting s2i pipeline with VERSION=%s", tag)
				pipelineRunName := opc.StartPipeline(
					"s2i-go-pipeline",
					map[string]string{"VERSION": tag},
					map[string]string{"name=source": "claimName=shared-pvc"},
					ns,
					"--use-param-defaults", "--prefix-name", "s2i-go-pipeline-run-"+tag,
				)
				pipelines.ValidatePipelineRun(sharedClients, pipelineRunName, "successful", ns)
			}
		})
	})
