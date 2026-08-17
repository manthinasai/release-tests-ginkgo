package pac_test

import (
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega
	gitlab "github.com/xanzy/go-gitlab"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pac"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Pipelines As Code GitLab tests: PIPELINES-30", func() {

	// =========================================================================
	// =========================================================================
	Describe("Configure PAC with push and pull_request events: PIPELINES-30-TC01", Ordered, ContinueOnFailure, Label("pac", "sanity", "e2e"), func() {
		var (
			gitlabClient *gitlab.Client
			project      *gitlab.Project
			projectID    int
			mrID         int
			smeeURL      string
			namespace    string
		)

		BeforeAll(func() {
			namespace = store.Namespace()
			lastNamespace = namespace

			gitlabClient = pac.InitGitLabClient(namespace)
			pac.SetGitLabClient(gitlabClient)

			var err error
			smeeURL, err = pac.SetupSmeeDeployment(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred(), "failed to setup Smee deployment")

			k8s.ValidateDeployments(sharedClients, namespace, "gosmee-client")

			project, err = pac.SetupGitLabProject(sharedClients, namespace, smeeURL)
			Expect(err).NotTo(HaveOccurred(), "failed to setup GitLab project")
			projectID = project.ID

			DeferCleanup(func() {
				err := pac.CleanupPAC(sharedClients, namespace, projectID, "gosmee-client")
				if err != nil {
					log.Printf("CleanupPAC warning: %v", err)
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

		It("should configure preview changes with both files", func() {
			var err error
			mrID, err = pac.ConfigurePreviewChanges(projectID)
			Expect(err).NotTo(HaveOccurred())
			Expect(mrID).To(BeNumerically(">", 0))
		})

		It("should validate pull_request PipelineRun succeeds", func() {
			pipelineName, err := pac.GetPipelineNameFromMR(sharedClients, namespace, projectID, mrID)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
		})

		It("should trigger push event on main branch", func() {
			err := pac.TriggerPushOnForkMain(projectID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate push PipelineRun succeeds", func() {
			pipelineName, err := pac.GetPushPipelineNameFromMain(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
		})
	})

	// =========================================================================
	// =========================================================================
	Describe("Configure PAC with on-label annotation: PIPELINES-30-TC02", Ordered, ContinueOnFailure, Label("pac", "e2e"), func() {
		var (
			gitlabClient *gitlab.Client
			project      *gitlab.Project
			projectID    int
			mrID         int
			smeeURL      string
			namespace    string
		)

		BeforeAll(func() {
			namespace = store.Namespace()
			lastNamespace = namespace

			gitlabClient = pac.InitGitLabClient(namespace)
			pac.SetGitLabClient(gitlabClient)

			var err error
			smeeURL, err = pac.SetupSmeeDeployment(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred(), "failed to setup Smee deployment")

			k8s.ValidateDeployments(sharedClients, namespace, "gosmee-client")

			project, err = pac.SetupGitLabProject(sharedClients, namespace, smeeURL)
			Expect(err).NotTo(HaveOccurred(), "failed to setup GitLab project")
			projectID = project.ID

			DeferCleanup(func() {
				err := pac.CleanupPAC(sharedClients, namespace, projectID, "gosmee-client")
				if err != nil {
					log.Printf("CleanupPAC warning: %v", err)
				}
			})
		})

		It("should generate pull_request PipelineRun YAML", func() {
			err := pac.GeneratePipelineRunYaml("pull_request", "main")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update on-label annotation with [bug]", func() {
			_, err := pac.UpdateAnnotation("pipelinesascode.tekton.dev/on-label", "[bug]")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should configure preview changes", func() {
			var err error
			mrID, err = pac.ConfigurePreviewChanges(projectID)
			Expect(err).NotTo(HaveOccurred())
			Expect(mrID).To(BeNumerically(">", 0))
		})

		It("should have 0 pipelineruns within 10 seconds", func() {
			Consistently(func() int {
				prlist, err := sharedClients.PipelineRunClient.List(sharedClients.Ctx, metav1.ListOptions{})
				if err != nil {
					return -1
				}
				return len(prlist.Items)
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Equal(0))
		})

		It("should add label bug to the merge request", func() {
			err := pac.AddLabel(projectID, mrID, "bug", "red", "Identify a Issue")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate pull_request PipelineRun succeeds", func() {
			pipelineName, err := pac.GetPipelineNameFromMR(sharedClients, namespace, projectID, mrID)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
		})
	})

	// =========================================================================
	// =========================================================================
	Describe("Configure PAC with on-comment annotation: PIPELINES-30-TC03", Ordered, ContinueOnFailure, Label("pac", "e2e"), func() {
		var (
			gitlabClient *gitlab.Client
			project      *gitlab.Project
			projectID    int
			mrID         int
			smeeURL      string
			namespace    string
		)

		BeforeAll(func() {
			namespace = store.Namespace()
			lastNamespace = namespace

			gitlabClient = pac.InitGitLabClient(namespace)
			pac.SetGitLabClient(gitlabClient)

			var err error
			smeeURL, err = pac.SetupSmeeDeployment(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred(), "failed to setup Smee deployment")

			k8s.ValidateDeployments(sharedClients, namespace, "gosmee-client")

			project, err = pac.SetupGitLabProject(sharedClients, namespace, smeeURL)
			Expect(err).NotTo(HaveOccurred(), "failed to setup GitLab project")
			projectID = project.ID

			DeferCleanup(func() {
				err := pac.CleanupPAC(sharedClients, namespace, projectID, "gosmee-client")
				if err != nil {
					log.Printf("CleanupPAC warning: %v", err)
				}
			})
		})

		It("should generate pull_request PipelineRun YAML", func() {
			err := pac.GeneratePipelineRunYaml("pull_request", "main")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update on-comment annotation with ^/hello-world", func() {
			_, err := pac.UpdateAnnotation("pipelinesascode.tekton.dev/on-comment", "^/hello-world")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should configure preview changes", func() {
			var err error
			mrID, err = pac.ConfigurePreviewChanges(projectID)
			Expect(err).NotTo(HaveOccurred())
			Expect(mrID).To(BeNumerically(">", 0))
		})

		It("should validate first pull_request PipelineRun succeeds", func() {
			pipelineName, err := pac.GetPipelineNameFromMR(sharedClients, namespace, projectID, mrID)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
		})

		It("should add comment /hello-world in MR", func() {
			err := pac.AddComment(projectID, mrID, "/hello-world")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have 2 pipelineruns within 10 seconds", func() {
			Eventually(func() int {
				prlist, err := sharedClients.PipelineRunClient.List(sharedClients.Ctx, metav1.ListOptions{})
				if err != nil {
					return -1
				}
				return len(prlist.Items)
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Equal(2))
		})

		It("should validate second pull_request PipelineRun succeeds", func() {
			pipelineName, err := pac.GetPipelineNameFromMR(sharedClients, namespace, projectID, mrID)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
		})
	})

	// =========================================================================
	// =========================================================================
	Describe("Configure PAC with GitOps tag commands: PIPELINES-30-TC04", Ordered, ContinueOnFailure, Label("pac", "e2e"), func() {
		var (
			gitlabClient *gitlab.Client
			project      *gitlab.Project
			projectID    int
			smeeURL      string
			namespace    string
		)

		const tagName = "v1.0.0"

		BeforeAll(func() {
			namespace = store.Namespace()
			lastNamespace = namespace

			gitlabClient = pac.InitGitLabClient(namespace)
			pac.SetGitLabClient(gitlabClient)

			var err error
			smeeURL, err = pac.SetupSmeeDeployment(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred(), "failed to setup Smee deployment")

			k8s.ValidateDeployments(sharedClients, namespace, "gosmee-client")

			project, err = pac.SetupGitLabProject(sharedClients, namespace, smeeURL)
			Expect(err).NotTo(HaveOccurred(), "failed to setup GitLab project")
			projectID = project.ID

			DeferCleanup(func() {
				err := pac.CleanupPAC(sharedClients, namespace, projectID, "gosmee-client")
				if err != nil {
					log.Printf("CleanupPAC warning: %v", err)
				}
			})
		})

		It("should generate push PipelineRun YAML", func() {
			err := pac.GeneratePipelineRunYaml("push", "main")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update push on-target-branch annotation to [refs/tags/*]", func() {
			err := pac.UpdatePushOnTargetBranch("[refs/tags/*]")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should trigger push event on main branch", func() {
			err := pac.TriggerPushOnForkMain(projectID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create tag v1.0.0 on main branch", func() {
			err := pac.CreateTagOnBranch(projectID, tagName, "main")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have 1 pipelinerun within 60 seconds", func() {
			Eventually(func() int {
				prlist, err := sharedClients.PipelineRunClient.List(sharedClients.Ctx, metav1.ListOptions{})
				if err != nil {
					return -1
				}
				return len(prlist.Items)
			}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Equal(1))
		})

		It("should add GitOps comment /cancel tag:v1.0.0 on tag v1.0.0", func() {
			err := pac.AddCommitCommentOnTag(projectID, "/cancel tag:"+tagName, tagName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should wait for latest PipelineRun to be cancelled", func() { //nolint:misspell
			pipelineName, err := pac.GetPushPipelineNameFromMain(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred())
			pipelines.WaitForPipelineRunCancelled(sharedClients, pipelineName, namespace)
		})

		It("should add GitOps /test comment for latest PipelineRun on tag v1.0.0", func() {
			err := pac.AddTestCommentForLatestPipelineRunOnTag(projectID, tagName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have 2 pipelineruns within 60 seconds", func() {
			Eventually(func() int {
				prlist, err := sharedClients.PipelineRunClient.List(sharedClients.Ctx, metav1.ListOptions{})
				if err != nil {
					return -1
				}
				return len(prlist.Items)
			}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Equal(2))
		})

		It("should add GitOps /cancel comment for latest PipelineRun on tag v1.0.0", func() {
			err := pac.AddCancelCommentForLatestPipelineRunOnTag(projectID, tagName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should wait for second PipelineRun to be cancelled", func() { //nolint:misspell
			pipelineName, err := pac.GetPushPipelineNameFromMain(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred())
			pipelines.WaitForPipelineRunCancelled(sharedClients, pipelineName, namespace)
		})

		It("should add GitOps comment /test tag:v1.0.0 on tag v1.0.0", func() {
			err := pac.AddCommitCommentOnTag(projectID, "/test tag:"+tagName, tagName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have 3 pipelineruns within 60 seconds", func() {
			Eventually(func() int {
				prlist, err := sharedClients.PipelineRunClient.List(sharedClients.Ctx, metav1.ListOptions{})
				if err != nil {
					return -1
				}
				return len(prlist.Items)
			}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Equal(3))
		})

		It("should add GitOps comment /cancel tag:v1.0.0 on tag v1.0.0", func() {
			err := pac.AddCommitCommentOnTag(projectID, "/cancel tag:"+tagName, tagName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add GitOps comment /retest tag:v1.0.0 on tag v1.0.0", func() {
			err := pac.AddCommitCommentOnTag(projectID, "/retest tag:"+tagName, tagName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate push PipelineRun succeeds", func() {
			pipelineName, err := pac.GetPushPipelineNameFromMain(sharedClients, namespace)
			Expect(err).NotTo(HaveOccurred())
			pipelines.ValidatePipelineRun(sharedClients, pipelineName, "success", namespace)
		})
	})
})
