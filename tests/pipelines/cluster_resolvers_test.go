package pipelines_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// Cluster resolvers require cross-namespace setup (tasks in one namespace, pipelines in another)
// Use sync.Once to create these shared namespaces only once across all tests in this file
var (
	clusterResolverSetupOnce      sync.Once
	clusterResolverNamespaceSetup bool // Track if namespaces were created
)

func setupClusterResolverNamespaces() {
	clusterResolverSetupOnce.Do(func() {
		// Create separate projects for tasks and pipelines (cross-namespace resolution precondition)
		// Use IgnoreErrors in case projects already exist from a previous test run
		oc.CreateNewProjectIgnoreErrors("releasetest-tasks")
		oc.Apply("testdata/resolvers/tasks/resolver-task.yaml", "releasetest-tasks")
		oc.Apply("testdata/resolvers/tasks/resolver-task2.yaml", "releasetest-tasks")
		oc.CreateNewProjectIgnoreErrors("releasetest-pipelines")
		oc.Apply("testdata/resolvers/pipelines/resolver-pipeline.yaml", "releasetest-pipelines")
		clusterResolverNamespaceSetup = true // Mark as created
	})
}

// CleanupClusterResolverNamespaces deletes the shared namespaces created for cluster resolver tests.
// Called from suite_test.go AfterSuite to match Gauge spec Teardown section.
// Only cleans up if namespaces were actually created.
func CleanupClusterResolverNamespaces() {
	if clusterResolverNamespaceSetup {
		oc.DeleteProjectIgnoreErrors("releasetest-tasks")
		oc.DeleteProjectIgnoreErrors("releasetest-pipelines")
	}
}

var _ = Describe("Cluster resolver cross-namespace resolution: PIPELINES-23-TC01", Label("e2e", "sanity"), func() {
	var ns string

	BeforeEach(func() {
		setupClusterResolverNamespaces()
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should resolve tasks from different namespace", func() {
		oc.Create("testdata/resolvers/pipelineruns/resolver-pipelinerun.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "resolver-pipelinerun", "successful", ns)
	})
})

var _ = Describe("Cluster resolver same-namespace resolution: PIPELINES-23-TC02", Label("e2e"), func() {
	var ns string

	BeforeEach(func() {
		setupClusterResolverNamespaces()
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should resolve tasks from same namespace", func() {
		oc.Create("testdata/resolvers/pipelines/resolver-pipeline-same-ns.yaml", ns)
		oc.Create("testdata/resolvers/pipelineruns/resolver-pipelinerun-same-ns.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "resolver-pipelinerun-same-ns", "successful", ns)
	})
})
