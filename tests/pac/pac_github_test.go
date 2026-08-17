package pac_test

import (
	"log"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pac"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Pipelines As Code GitHub tests: PIPELINES-35", func() {

	// =========================================================================
	// =========================================================================
	Describe("Configure PAC in GitHub Project: PIPELINES-35-TC01", Ordered, ContinueOnFailure, Label("pac", "sanity", "e2e"), func() {
		var (
			owner               string
			repoName            string
			smeeURL             string
			namespace           string
			prNumber            int
			lastPipelineRunName string
		)

		BeforeAll(func() {
			namespace = store.Namespace()
			lastNamespace = namespace

			var err error

			err = pac.AssertPACInfoInstall()
			Expect(err).NotTo(HaveOccurred(), "PAC info install validation failed")

			pac.SetGitHubClient(pac.InitGitHubClient())

			smeeURL, err = pac.SetupSmeeDeployment(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred(), "failed to setup Smee deployment")

			k8s.ValidateDeployments(sharedClients, namespace, "gosmee-client")

			owner, repoName, err = pac.SetupGitHubProject(sharedClients, namespace, smeeURL)
			Expect(err).NotTo(HaveOccurred(), "failed to setup GitHub project")

			DeferCleanup(func() {
				cleanupErr := pac.CleanupPACGitHub(sharedClients, namespace, "gosmee-client", owner, repoName)
				if cleanupErr != nil {
					log.Printf("CleanupPACGitHub warning: %v", cleanupErr)
				}
			})
		})

		It("should generate pull_request PipelineRun YAML", func() {
			err := pac.GeneratePipelineRunYaml("pull_request", "main")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should generate push PipelineRun YAML", func() {
			err := pac.GeneratePipelineRunYaml("push", "main")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should configure PipelineRun by creating a GitHub PR with pipeline definitions", func() {
			var prURL string
			var err error
			prURL, prNumber, err = pac.ConfigurePreviewChangesGitHub(owner, repoName)
			Expect(err).NotTo(HaveOccurred())
			Expect(prNumber).To(BeNumerically(">", 0))
			log.Printf("Pull Request created: %s", prURL)
		})

		It("should validate pull_request PipelineRun for success", func() {
			pipelineName, err := pac.WaitForNewPipelineRunName(sharedClients, namespace, "")
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
			lastPipelineRunName = pipelineName
		})

		It("should trigger push event on main branch by merging the PR", func() {
			err := pac.TriggerPushOnGitHubMain(owner, repoName, prNumber)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate push PipelineRun for success", func() {
			pipelineName, err := pac.WaitForNewPipelineRunName(sharedClients, namespace, lastPipelineRunName)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
		})
	})
})
