package pipelines_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Test git resolver functionality: PIPELINES-24-TC01", Label("e2e", "sanity"), func() {
	var ns string

	BeforeEach(func() {
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should resolve tasks from git repository", func() {
		oc.Create("testdata/resolvers/pipelineruns/git-resolver-pipelinerun.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-resolver-pipelinerun", "successful", ns)
	})
})

var _ = Describe("Test git resolver with authentication and token: PIPELINES-24-TC02", Label("e2e"), func() {
	var ns string

	BeforeEach(func() {
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should resolve tasks from private git repository with token", func() {
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			Skip("GITHUB_TOKEN not set -- skipping private repo resolver test")
		}

		// Create secret with GitHub token for private repo access
		// Key must be "private-repo-token" to match gitTokenKey in YAML
		cmd.MustSucceed("oc", "create", "secret", "generic", "private-repo-auth-secret",
			"--from-literal=private-repo-token="+token, "-n", ns)
		DeferCleanup(func() {
			cmd.Run("oc", "delete", "secret", "private-repo-auth-secret", "-n", ns)
		})

		oc.Create("testdata/resolvers/pipelineruns/git-resolver-pipelinerun-private.yaml", ns)
		oc.Create("testdata/resolvers/pipelineruns/git-resolver-pipelinerun-private-token-auth.yaml", ns)
		oc.Create("testdata/resolvers/pipelineruns/git-resolver-pipelinerun-private-url.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "git-resolver-pipelinerun-private", "successful", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-resolver-pipelinerun-private-token-auth", "successful", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-resolver-pipelinerun-private-url", "successful", ns)
	})
})
