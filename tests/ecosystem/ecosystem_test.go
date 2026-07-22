package ecosystem_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

var oc = occmd.OC{}

// ========================================================================
// ========================================================================

// ecoNsCounter provides unique namespace names per test. Incremented each time
// createTestNamespace is called within this file.
var ecoNsCounter int

// createTestNamespace creates a new OpenShift project with a unique name derived from
// the given prefix and returns the namespace name. The namespace name includes the
// Ginkgo parallel process index and an incrementing counter to avoid collisions.
func createTestNamespace(prefix string) string {
	ecoNsCounter++
	ns := fmt.Sprintf("%s-%d-%d", prefix, GinkgoParallelProcess(), ecoNsCounter)
	oc.CreateNewNamespace(ns)
	return ns
}

// PIPELINES-29
// -----------------------------------------------------------------------
// DescribeTable 1: Ecosystem Task Pipelines (simple create-verify pattern)
//
// These tests follow the most common ecosystem pattern:
//  1. Create a unique namespace and register cleanup
//  2. Create YAML resources (pipelines, PVCs, pipelineruns) via oc
//  3. Verify pipelinerun reaches expected status
//
// CRITICAL: Entry parameters are evaluated at TREE CONSTRUCTION TIME.
//   - Only string literals, slice literals, and constant values are used as Entry args.
//   - Dynamic values (namespace, client handles) are created INSIDE the table body
//     function at spec execution time, never passed through Entry.
//   - The `resources []string` parameter is a slice literal, safe for tree-construction.
//
// -----------------------------------------------------------------------
var _ = DescribeTable("Ecosystem Task Pipelines",
	func(pipelineRunName string, resources []string, expectedStatus string) {
		ns := createTestNamespace("eco-simple")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)

		// Initialize typed clients scoped to the new namespace
		sharedClients.NewClientSet(ns)

		// Wait for the pipeline SA to be created by the operator RBAC reconciler
		// before applying any PipelineRun that references it.
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")

		for _, resource := range resources {
			oc.Create(resource, ns)
		}

		// Verify the pipelinerun reaches the expected status
		pipelines.ValidatePipelineRun(sharedClients, pipelineRunName, expectedStatus, ns)
	},

	// Labels applied to every Entry in this table
	Label("ecosystem", "e2e"),

	// PIPELINES-29-TC01
	Entry("buildah pipelinerun", Label("sanity", "buildah"),
		"buildah-run",
		[]string{
			"testdata/ecosystem/pipelines/buildah.yaml",
			"testdata/pvc/pvc.yaml",
			"testdata/ecosystem/pipelineruns/buildah.yaml",
		},
		"successful"),

	// PIPELINES-29-TC03
	Entry("git-cli pipelinerun", Label("git-cli"),
		"git-cli-run",
		[]string{
			"testdata/ecosystem/pipelines/git-cli.yaml",
			"testdata/pvc/pvc.yaml",
			"testdata/ecosystem/pipelineruns/git-cli.yaml",
		},
		"successful"),

	// PIPELINES-29-TC08
	Entry("openshift-client pipelinerun", Label("openshift-client"),
		"openshift-client-run",
		[]string{
			"testdata/ecosystem/pipelineruns/openshift-client.yaml",
		},
		"successful"),

	// PIPELINES-29-TC09
	Entry("skopeo-copy pipelinerun", Label("skopeo-copy"),
		"skopeo-copy-run",
		[]string{
			"testdata/ecosystem/pipelineruns/skopeo-copy.yaml",
		},
		"successful"),

	// PIPELINES-29-TC10
	Entry("tkn pipelinerun", Label("tkn"),
		"tkn-run",
		[]string{
			"testdata/ecosystem/pipelineruns/tkn.yaml",
		},
		"successful"),

	// PIPELINES-29-TC13
	Entry("maven pipelinerun", Label("maven"),
		"maven-run",
		[]string{
			"testdata/ecosystem/pipelines/maven.yaml",
			"testdata/pvc/pvc.yaml",
			"testdata/ecosystem/configmaps/maven-settings.yaml",
			"testdata/ecosystem/pipelineruns/maven.yaml",
		},
		"successful"),

	// PIPELINES-29-TC14
	Entry("step action resolvers", Label("sanity"),
		"git-clone-stepaction-run",
		[]string{
			"testdata/ecosystem/tasks/git-clone-stepaction.yaml",
			"testdata/pvc/pvc.yaml",
			"testdata/ecosystem/pipelineruns/git-clone-stepaction.yaml",
		},
		"successful"),

	// PIPELINES-29-TC21
	Entry("opc task pipelinerun", Label("sanity", "opc"),
		"opc-task-run",
		[]string{
			"testdata/ecosystem/pipelines/opc-task.yaml",
			"testdata/ecosystem/pipelineruns/opc-task.yaml",
		},
		"successful"),
)

// -----------------------------------------------------------------------
// DescribeTable 2: Ecosystem Task Pipelines with Extra Verification
//
// These tests extend the simple create-verify pattern with a post-verification
// callback. After the pipelinerun succeeds, the postVerify function executes
// additional validation (log checks, deployment readiness, etc.).
//
// The postVerify parameter is a function literal defined inline in each Entry.
// Function literals are safe for tree-construction time since they are not
// variables initialized at runtime -- they are compile-time constants.
// -----------------------------------------------------------------------
var _ = DescribeTable("Ecosystem Task Pipelines with Extra Verification",
	func(pipelineRunName string, resources []string, expectedStatus string, postVerify func(ns string)) {
		ns := createTestNamespace("eco-extra")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		// Wait for the pipeline SA to be created by the operator RBAC reconciler
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")

		for _, resource := range resources {
			oc.Create(resource, ns)
		}

		pipelines.ValidatePipelineRun(sharedClients, pipelineRunName, expectedStatus, ns)

		// Run post-verification callback
		if postVerify != nil {
			postVerify(ns)
		}
	},

	Label("ecosystem", "e2e"),

	// PIPELINES-29-TC11
	Entry("tkn pac pipelinerun", Label("tkn"),
		"tkn-pac-run",
		[]string{
			"testdata/ecosystem/pipelineruns/tkn-pac.yaml",
		},
		"successful",
		func(ns string) { pipelines.CheckLogVersion(sharedClients, "tkn-pac", ns) }),

	// PIPELINES-29-TC12
	Entry("tkn version pipelinerun", Label("tkn"),
		"tkn-version-run",
		[]string{
			"testdata/ecosystem/pipelineruns/tkn-version.yaml",
		},
		"successful",
		func(ns string) { pipelines.CheckLogVersion(sharedClients, "tkn", ns) }),

	// PIPELINES-29-TC17
	Entry("helm-upgrade-from-repo pipelinerun", Label("helm"),
		"helm-upgrade-from-repo-run",
		[]string{
			"testdata/ecosystem/pipelines/helm-upgrade-from-repo.yaml",
			"testdata/pvc/pvc.yaml",
			"testdata/ecosystem/pipelineruns/helm-upgrade-from-repo.yaml",
		},
		"successful",
		func(ns string) { k8s.ValidateDeployments(sharedClients, ns, "test-hello-world") }),

	// PIPELINES-29-TC18
	Entry("helm-upgrade-from-source pipelinerun", Label("helm"),
		"helm-upgrade-from-source-run",
		[]string{
			"testdata/ecosystem/pipelines/helm-upgrade-from-source.yaml",
			"testdata/pvc/pvc.yaml",
			"testdata/ecosystem/pipelineruns/helm-upgrade-from-source.yaml",
		},
		"successful",
		func(ns string) { k8s.ValidateDeployments(sharedClients, ns, "test-hello-world") }),
)

// buildah-ns uses user-namespace (rootless) mode which requires the pipeline SA
// to have the pipelines-scc SCC bound so the container gets a valid UID map.
var _ = Describe("Ecosystem Special Tasks", Label("ecosystem", "e2e"), func() {

	// PIPELINES-29-TC20
	It("buildah-ns pipelinerun", Label("sanity", "buildah-ns"), func() {
		// buildah-ns fails on OCP 4.20+ due to a product bug — skip until fixed.
		// https://issues.redhat.com/browse/SRVKP-11139
		k8s.SkipIfOCPVersionGTE(sharedClients, 20, "SRVKP-11139", "buildah-ns task fails reading /proc/0/uid_map")

		ns := createTestNamespace("eco-buildah-ns")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		// The Tekton operator creates the 'pipeline' SA and pipelines-scc-rolebinding
		// asynchronously. Wait for the SA to exist before granting the SCC so the
		// oc adm policy command doesn't silently fail or error on a missing subject.
		k8s.WaitForServiceAccount(sharedClients, ns, "pipeline")

		// buildah-ns runs in user-namespace (rootless) mode.
		// Grant pipelines-scc to the pipeline SA so OpenShift writes valid
		// UID/GID mappings into the container's /proc/<pid>/uid_map.
		oc.AddSCCToServiceAccount("pipelines-scc", "pipeline", ns)

		oc.Create("testdata/ecosystem/pipelines/buildah-ns.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/buildah-ns.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "buildah-ns-run", "successful", ns)
	})

	// pull-request pipeline requires copying a secret from the
	// openshift-pipelines namespace before creating the pipelinerun.
	// PIPELINES-29-TC19
	It("pull-request pipelinerun", Label("pull-request"), func() {
		ns := createTestNamespace("eco-pull-request")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		// Copy github-auth-secret from openshift-pipelines namespace
		if !oc.SecretExists("github-auth-secret", "openshift-pipelines") {
			Skip("github-auth-secret not found in openshift-pipelines namespace")
		}
		oc.CopySecret("github-auth-secret", "openshift-pipelines", ns)

		oc.Create("testdata/ecosystem/pipelines/pull-request.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/pull-request.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "pull-request-pipeline-run", "successful", ns)
	})
})

// PIPELINES-29-TC02
var _ = Describe("buildah disconnected pipelinerun", Label("ecosystem", "e2e", "disconnected", "buildah"), func() {
	It("should create and verify buildah disconnected pipelinerun", func() {
		if !config.Flags.IsDisconnected {
			Skip("requires disconnected cluster (set IS_DISCONNECTED=true)")
		}

		ns := createTestNamespace("eco-buildah-disconnected")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/buildah.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/buildah-disconnected.yaml", ns)

		pipelines.ValidatePipelineRun(sharedClients, "buildah-disconnected-run", "successful", ns)
	})
})
