package ecosystem_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

// -----------------------------------------------------------------------
// Secret-Link Tests (4 tests)
//
// These tests require creating secrets and linking them to service accounts
// before running the pipelinerun. Each test has varying setup (different
// resources, different service accounts), so individual It blocks are used
// instead of a DescribeTable.
//
// Pattern: create namespace -> create pipeline + PVC + secret -> link secret
//          to SA -> create pipelinerun -> validate pipelinerun
// -----------------------------------------------------------------------

// PIPELINES-29
var _ = Describe("Ecosystem Secret-Link Tasks", Label("ecosystem", "e2e"), func() {

	// PIPELINES-29-TC04
	It("git-cli read private repo pipelinerun", Label("git-cli"), func() {
		ns := createTestNamespace("eco-git-cli-priv")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		// Create resources
		oc.Create("testdata/ecosystem/pipelines/git-cli-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)

		// Link secret to pipeline SA
		oc.LinkSecretToSA("ssh-key", "pipeline", ns)

		// Create pipelinerun
		oc.Create("testdata/ecosystem/pipelineruns/git-cli-read-private.yaml", ns)

		// Verify
		pipelines.ValidatePipelineRun(sharedClients, "git-cli-read-private-run", "successful", ns)
	})

	// PIPELINES-29-TC05
	It("git-cli read private repo using different SA", Label("git-cli"), func() {
		ns := createTestNamespace("eco-git-cli-sa")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/git-cli-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)
		oc.Create("testdata/ecosystem/serviceaccount/ssh-sa.yaml", ns)
		oc.Create("testdata/ecosystem/rolebindings/ssh-sa-scc.yaml", ns)

		// Link secret to custom SA
		oc.LinkSecretToSA("ssh-key", "ssh-sa", ns)

		oc.Create("testdata/ecosystem/pipelineruns/git-cli-read-private-sa.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "git-cli-read-private-sa-run", "successful", ns)
	})

	// PIPELINES-29-TC06
	It("git-clone read private repo taskrun", Label("sanity", "git-clone"), func() {
		ns := createTestNamespace("eco-git-clone-priv")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/git-clone-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)

		// Link secret to pipeline SA
		oc.LinkSecretToSA("ssh-key", "pipeline", ns)

		oc.Create("testdata/ecosystem/pipelineruns/git-clone-read-private.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "git-clone-read-private-pipeline-run", "successful", ns)
	})

	// PIPELINES-29-TC07
	It("git-clone read private repo using different SA", Label("git-clone"), func() {
		ns := createTestNamespace("eco-git-clone-sa")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/git-clone-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)
		oc.Create("testdata/ecosystem/serviceaccount/ssh-sa.yaml", ns)
		oc.Create("testdata/ecosystem/rolebindings/ssh-sa-scc.yaml", ns)

		// Link secret to custom SA
		oc.LinkSecretToSA("ssh-key", "ssh-sa", ns)

		oc.Create("testdata/ecosystem/pipelineruns/git-clone-read-private-sa.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "git-clone-read-private-pipeline-sa-run", "successful", ns)
	})
})

// -----------------------------------------------------------------------
// Cache Pipeline Tests (2 tests)
//
// These tests start pipelines via opc CLI with specific params and validate
// log output. The cache-upload stepaction is run twice to verify that the
// second run detects the already-uploaded cache.
//
// Workspace format for opc.StartPipeline: the map key is "name=<workspace>"
// and value is "claimName=<pvc>", matching the Gauge step format
// "name=source,claimName=shared-pvc" which is split on comma.
// -----------------------------------------------------------------------

var _ = Describe("Ecosystem Cache Tasks", Label("ecosystem", "e2e", "cache"), func() {

	// PIPELINES-29-TC15
	It("cache-upload stepaction", Label("sanity"), func() {
		ns := createTestNamespace("eco-cache-upload")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		// Create pipeline and PVC
		oc.Create("testdata/ecosystem/pipelines/cache-stepactions-python.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		params := map[string]string{"revision": "release-v1.17"}
		workspaces := map[string]string{"name=source": "claimName=shared-pvc"}

		// First run -- should upload cache
		prName1 := opc.StartPipeline("caches-python-pipeline", params, workspaces, ns, "--use-param-defaults")
		pipelines.ValidatePipelineRun(sharedClients, prName1, "successful", ns)

		// Validate first run logs contain upload message
		logs1 := cmd.MustSucceed("oc", "logs", "-l",
			"tekton.dev/pipelineRun="+prName1+",tekton.dev/pipelineTask=cache-upload",
			"-n", ns).Stdout()
		Expect(strings.ToLower(logs1)).To(ContainSubstring("upload /workspace/source/cache/lib content to oci image"))

		// Second run -- should skip upload (cache already exists)
		prName2 := opc.StartPipeline("caches-python-pipeline", params, workspaces, ns, "--use-param-defaults")
		pipelines.ValidatePipelineRun(sharedClients, prName2, "successful", ns)

		logs2 := cmd.MustSucceed("oc", "logs", "-l",
			"tekton.dev/pipelineRun="+prName2+",tekton.dev/pipelineTask=cache-upload",
			"-n", ns).Stdout()
		Expect(strings.ToLower(logs2)).To(ContainSubstring("no need to upload cache"))
	})

	// PIPELINES-29-TC16
	It("cache upload with revision change", func() {
		ns := createTestNamespace("eco-cache-rev")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/cache-stepactions-python.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		workspaces := map[string]string{"name=source": "claimName=shared-pvc"}

		// First run with revision release-v1.17
		params1 := map[string]string{"revision": "release-v1.17"}
		prName1 := opc.StartPipeline("caches-python-pipeline", params1, workspaces, ns, "--use-param-defaults")
		pipelines.ValidatePipelineRun(sharedClients, prName1, "successful", ns)

		logs1 := cmd.MustSucceed("oc", "logs", "-l",
			"tekton.dev/pipelineRun="+prName1+",tekton.dev/pipelineTask=cache-upload",
			"-n", ns).Stdout()
		Expect(strings.ToLower(logs1)).To(ContainSubstring("upload /workspace/source/cache/lib content to oci image"))

		// Second run with different revision (master) -- should upload again
		params2 := map[string]string{"revision": "master"}
		prName2 := opc.StartPipeline("caches-python-pipeline", params2, workspaces, ns, "--use-param-defaults")
		pipelines.ValidatePipelineRun(sharedClients, prName2, "successful", ns)

		logs2 := cmd.MustSucceed("oc", "logs", "-l",
			"tekton.dev/pipelineRun="+prName2+",tekton.dev/pipelineTask=cache-upload",
			"-n", ns).Stdout()
		Expect(strings.ToLower(logs2)).To(ContainSubstring("upload /workspace/source/cache/lib content to oci image"))
	})
})
