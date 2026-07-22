package triggers_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/triggers"
)

var _ = Describe("Tutorial", Label("triggers"), func() {

	It("Run pipelines tutorials", Label("e2e", "integration", "non-admin", "pipelines", "tutorial", "skip-4.14"), func() {
		ns := config.TargetNamespace
		lastNamespace = ns

		// Create pipeline tutorial resources from remote URLs
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/01_apply_manifest_task.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/02_update_deployment_task.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/03_persistent_volume_claim.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/04_pipeline.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/02_pipelinerun/01_build_deploy_api_pipelinerun.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/02_pipelinerun/02_build_deploy_ui_pipelinerun.yaml", ns)

		// Verify both pipelineruns succeed
		pipelines.ValidatePipelineRun(sharedClients, "build-deploy-api-pipelinerun", "successful", ns)
		pipelines.ValidatePipelineRun(sharedClients, "build-deploy-ui-pipelinerun", "successful", ns)

		// Get route URL of pipelines-vote-ui
		routeURL := cmd.MustSucceed("oc", "-n", ns, "get", "route", "pipelines-vote-ui", "--template='http://{{.spec.host}}'").Stdout()
		routeURL = strings.Trim(routeURL, "'")

		// Wait for deployments
		k8s.ValidateDeployments(sharedClients, ns, "pipelines-vote-api")
		k8s.ValidateDeployments(sharedClients, ns, "pipelines-vote-ui")

		// Validate route URL contains expected content
		output := cmd.MustSucceedIncreasedTimeout(180*time.Second, "curl", "-kL", routeURL).Stdout()
		Expect(output).To(ContainSubstring("Cat"), "route URL should contain expected tutorial content")
	})

	It("Run pipelines tutorial using triggers", Label("e2e", "integration", "triggers", "non-admin", "tutorial", "sanity", "skip-4.14"), func() {
		ns := config.TargetNamespace
		lastNamespace = ns

		// Create pipeline and trigger resources from remote URLs
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/01_apply_manifest_task.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/02_update_deployment_task.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/03_persistent_volume_claim.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/01_pipeline/04_pipeline.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/03_triggers/01_binding.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/03_triggers/02_template.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/03_triggers/03_trigger.yaml", ns)
		oc.CreateRemote("https://raw.githubusercontent.com/openshift/pipelines-tutorial/{OSP_TUTORIAL_BRANCH}/03_triggers/04_event_listener.yaml", ns)

		// Expose EventListener
		routeURL := triggers.ExposeEventListener(sharedClients, "vote-app", ns)

		// First push event: vote-api
		resp1, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push-vote-api.json", false)
		triggers.AssertElResponse(sharedClients, resp1, "vote-app", ns)
		pipelines.AssertNumberOfPipelineruns(ns, 1, 15)
		prname1, err := pipelines.GetLatestPipelinerun(sharedClients, ns)
		Expect(err).NotTo(HaveOccurred())
		pipelines.ValidatePipelineRun(sharedClients, prname1, "successful", ns)

		// Second push event: vote-ui
		resp2, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push-vote-ui.json", false)
		triggers.AssertElResponse(sharedClients, resp2, "vote-app", ns)
		pipelines.AssertNumberOfPipelineruns(ns, 2, 15)
		prname2, err := pipelines.GetLatestPipelinerun(sharedClients, ns)
		Expect(err).NotTo(HaveOccurred())
		pipelines.ValidatePipelineRun(sharedClients, prname2, "successful", ns)

		// Get route URL of pipelines-vote-ui
		voteUIRouteURL := cmd.MustSucceed("oc", "-n", ns, "get", "route", "pipelines-vote-ui", "--template='http://{{.spec.host}}'").Stdout()
		voteUIRouteURL = strings.Trim(voteUIRouteURL, "'")

		// Wait for vote-ui deployment
		k8s.ValidateDeployments(sharedClients, ns, "pipelines-vote-ui")

		// Validate route URL contains expected content
		output := cmd.MustSucceedIncreasedTimeout(180*time.Second, "curl", "-kL", voteUIRouteURL).Stdout()
		Expect(output).To(ContainSubstring("Cat"), "route URL should contain expected tutorial content")
	})
})
