package ecosystem_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

var oc = occmd.OC{}

var _ = Describe("buildah pipelinerun: PIPELINES-29-TC01", Label("ecosystem", "e2e", "sanity", "buildah"), func() {
	It("should create and verify buildah pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/buildah.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/buildah.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "buildah-run", "successful", ns)
	})
})

var _ = Describe("buildah disconnected pipelinerun: PIPELINES-29-TC02", Label("ecosystem", "e2e", "disconnected", "buildah"), func() {
	It("should create and verify buildah disconnected pipelinerun", func() {
		if !config.Flags.IsDisconnected {
			Skip("requires disconnected cluster (set IS_DISCONNECTED=true)")
		}
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/buildah.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/buildah-disconnected.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "buildah-disconnected-run", "successful", ns)
	})
})

var _ = Describe("git-cli pipelinerun: PIPELINES-29-TC03", Label("ecosystem", "e2e", "git-cli"), func() {
	It("should create and verify git-cli pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/git-cli.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/git-cli.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-cli-run", "successful", ns)
	})
})

var _ = Describe("git-cli read private repo pipelinerun: PIPELINES-29-TC04", Label("ecosystem", "e2e", "git-cli"), func() {
	It("should create and verify git-cli read private repo pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/git-cli-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)
		oc.LinkSecretToSA("ssh-key", "pipeline", ns)
		oc.Create("testdata/ecosystem/pipelineruns/git-cli-read-private.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-cli-read-private-run", "successful", ns)
	})
})

var _ = Describe("git-cli read private repo using different service account pipelinerun: PIPELINES-29-TC05", Label("ecosystem", "e2e", "git-cli"), func() {
	It("should create and verify git-cli read private repo pipelinerun using different SA", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/git-cli-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)
		oc.Create("testdata/ecosystem/serviceaccount/ssh-sa.yaml", ns)
		oc.Create("testdata/ecosystem/rolebindings/ssh-sa-scc.yaml", ns)
		oc.LinkSecretToSA("ssh-key", "ssh-sa", ns)
		oc.Create("testdata/ecosystem/pipelineruns/git-cli-read-private-sa.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-cli-read-private-sa-run", "successful", ns)
	})
})

// TC06
var _ = Describe("git-clone read private repo taskrun: PIPELINES-29-TC06", Label("ecosystem", "e2e", "sanity", "git-clone"), func() {
	It("should create and verify git-clone read private repo pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/git-clone-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)
		oc.LinkSecretToSA("ssh-key", "pipeline", ns)
		oc.Create("testdata/ecosystem/pipelineruns/git-clone-read-private.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-clone-read-private-pipeline-run", "successful", ns)
	})
})

var _ = Describe("git-clone read private repo using different service account taskrun: PIPELINES-29-TC07", Label("ecosystem", "e2e", "git-clone"), func() {
	It("should create and verify git-clone read private repo pipelinerun using different SA", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/git-clone-read-private.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/secrets/ssh-key.yaml", ns)
		oc.Create("testdata/ecosystem/serviceaccount/ssh-sa.yaml", ns)
		oc.Create("testdata/ecosystem/rolebindings/ssh-sa-scc.yaml", ns)
		oc.LinkSecretToSA("ssh-key", "ssh-sa", ns)
		oc.Create("testdata/ecosystem/pipelineruns/git-clone-read-private-sa.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-clone-read-private-pipeline-sa-run", "successful", ns)
	})
})

var _ = Describe("openshift-client pipelinerun: PIPELINES-29-TC08", Label("ecosystem", "e2e", "openshift-client"), func() {
	It("should create and verify openshift-client pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelineruns/openshift-client.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "openshift-client-run", "successful", ns)
	})
})

var _ = Describe("skopeo-copy pipelinerun: PIPELINES-29-TC09", Label("ecosystem", "e2e", "skopeo-copy"), func() {
	It("should create and verify skopeo-copy pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelineruns/skopeo-copy.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "skopeo-copy-run", "successful", ns)
	})
})

var _ = Describe("tkn pipelinerun: PIPELINES-29-TC10", Label("ecosystem", "e2e", "tkn"), func() {
	It("should create and verify tkn pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelineruns/tkn.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "tkn-run", "successful", ns)
	})
})

var _ = Describe("tkn pac pipelinerun: PIPELINES-29-TC11", Label("ecosystem", "e2e", "tkn"), func() {
	It("should create and verify tkn pac pipelinerun and check log version", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelineruns/tkn-pac.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "tkn-pac-run", "successful", ns)
		pipelines.CheckLogVersion(sharedClients, "tkn-pac", ns)
	})
})

var _ = Describe("tkn version pipelinerun: PIPELINES-29-TC12", Label("ecosystem", "e2e", "tkn"), func() {
	It("should create and verify tkn version pipelinerun and check log version", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelineruns/tkn-version.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "tkn-version-run", "successful", ns)
		pipelines.CheckLogVersion(sharedClients, "tkn", ns)
	})
})

var _ = Describe("maven pipelinerun: PIPELINES-29-TC13", Label("ecosystem", "e2e", "maven"), func() {
	It("should create and verify maven pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/maven.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/configmaps/maven-settings.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/maven.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "maven-run", "successful", ns)
	})
})

var _ = Describe("Test the functionality of step action resolvers: PIPELINES-29-TC14", Label("ecosystem", "e2e", "sanity"), func() {
	It("should create and verify git-clone stepaction pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/tasks/git-clone-stepaction.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/git-clone-stepaction.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "git-clone-stepaction-run", "successful", ns)
	})
})

var _ = Describe("Test the functionality of cache-upload stepaction: PIPELINES-29-TC15", Label("ecosystem", "e2e", "cache", "sanity"), func() {
	It("should upload cache and skip on second run", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/cache-stepactions-python.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		params := map[string]string{"revision": "release-v1.17"}
		workspaces := map[string]string{"name=source": "claimName=shared-pvc"}

		// First run — should upload cache
		prName1 := opc.StartPipeline("caches-python-pipeline", params, workspaces, ns, "--use-param-defaults")
		pipelines.ValidatePipelineRun(sharedClients, prName1, "successful", ns)
		logs1 := cmd.MustSucceed("oc", "logs", "-l",
			"tekton.dev/pipelineRun="+prName1+",tekton.dev/pipelineTask=cache-upload",
			"-n", ns).Stdout()
		Expect(strings.ToLower(logs1)).To(ContainSubstring("upload /workspace/source/cache/lib content to oci image"))

		// Second run — should skip upload (cache already exists)
		prName2 := opc.StartPipeline("caches-python-pipeline", params, workspaces, ns, "--use-param-defaults")
		pipelines.ValidatePipelineRun(sharedClients, prName2, "successful", ns)
		logs2 := cmd.MustSucceed("oc", "logs", "-l",
			"tekton.dev/pipelineRun="+prName2+",tekton.dev/pipelineTask=cache-upload",
			"-n", ns).Stdout()
		Expect(strings.ToLower(logs2)).To(ContainSubstring("no need to upload cache"))
	})
})

var _ = Describe("Validate cache uploads with change in revision: PIPELINES-29-TC16", Label("ecosystem", "e2e", "cache"), func() {
	It("should re-upload cache when revision changes", func() {
		ns := lastNamespace
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

		// Second run with different revision — should upload again
		params2 := map[string]string{"revision": "master"}
		prName2 := opc.StartPipeline("caches-python-pipeline", params2, workspaces, ns, "--use-param-defaults")
		pipelines.ValidatePipelineRun(sharedClients, prName2, "successful", ns)
		logs2 := cmd.MustSucceed("oc", "logs", "-l",
			"tekton.dev/pipelineRun="+prName2+",tekton.dev/pipelineTask=cache-upload",
			"-n", ns).Stdout()
		Expect(strings.ToLower(logs2)).To(ContainSubstring("upload /workspace/source/cache/lib content to oci image"))
	})
})

var _ = Describe("helm-upgrade-from-repo pipelinerun: PIPELINES-29-TC17", Label("ecosystem", "e2e", "helm"), func() {
	It("should create and verify helm-upgrade-from-repo pipelinerun and validate deployment", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/helm-upgrade-from-repo.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/helm-upgrade-from-repo.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "helm-upgrade-from-repo-run", "successful", ns)
		k8s.ValidateDeployments(sharedClients, ns, "test-hello-world")
	})
})

var _ = Describe("helm-upgrade-from-source pipelinerun: PIPELINES-29-TC18", Label("ecosystem", "e2e", "helm"), func() {
	It("should create and verify helm-upgrade-from-source pipelinerun and validate deployment", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/helm-upgrade-from-source.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/helm-upgrade-from-source.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "helm-upgrade-from-source-run", "successful", ns)
		k8s.ValidateDeployments(sharedClients, ns, "test-hello-world")
	})
})

var _ = Describe("pull-request pipelinerun: PIPELINES-29-TC19", Label("ecosystem", "e2e", "pull-request"), func() {
	It("should create and verify pull-request pipelinerun", func() {
		if !oc.SecretExists("github-auth-secret", "openshift-pipelines") {
			Skip("github-auth-secret not found in openshift-pipelines namespace")
		}
		ns := lastNamespace
		oc.CopySecret("github-auth-secret", "openshift-pipelines", ns)
		oc.Create("testdata/ecosystem/pipelines/pull-request.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/pull-request.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "pull-request-pipeline-run", "successful", ns)
	})
})

// buildah-ns uses user-namespace (rootless) mode which requires the pipeline SA
// to have the pipelines-scc SCC bound so the container gets a valid UID map.
var _ = Describe("buildah-ns pipelinerun: PIPELINES-29-TC20", Label("ecosystem", "e2e", "sanity", "buildah-ns"), func() {
	It("should create and verify buildah-ns pipelinerun", func() {
		// buildah-ns fails on OCP 4.20+ due to a product bug — skip until fixed.
		// https://issues.redhat.com/browse/SRVKP-11139
		k8s.SkipIfOCPVersionGTE(sharedClients, 20, "SRVKP-11139", "buildah-ns task fails reading /proc/0/uid_map")
		ns := lastNamespace
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")
		oc.AddSCCToServiceAccount("pipelines-scc", "pipeline", ns)
		oc.Create("testdata/ecosystem/pipelines/buildah-ns.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/buildah-ns.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "buildah-ns-run", "successful", ns)
	})
})

var _ = Describe("opc task pipelinerun: PIPELINES-29-TC21", Label("ecosystem", "e2e", "sanity", "opc"), func() {
	It("should create and verify opc task pipelinerun", func() {
		ns := lastNamespace
		oc.Create("testdata/ecosystem/pipelines/opc-task.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/opc-task.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "opc-task-run", "successful", ns)
	})
})
