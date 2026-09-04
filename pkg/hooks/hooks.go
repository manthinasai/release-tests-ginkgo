// Package hooks provides Ginkgo lifecycle hooks for test namespace management.
// It automatically creates unique namespaces per Describe block using container
// hierarchy tracking.
package hooks

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	"github.com/tektoncd/pipeline/pkg/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var oc = occmd.OC{}

// namespaceManager holds the state for automatic namespace management.
type namespaceManager struct {
	mu                sync.Mutex
	containerToNS     map[string]string // container path -> namespace
	containerFailed   map[string]bool   // container path -> has failure
	lastContainerPath string            // track container changes for cleanup
}

var globalNSManager = &namespaceManager{
	containerToNS:   make(map[string]string),
	containerFailed: make(map[string]bool),
}

// CleanupNamespaces should be called from your existing AfterSuite.
// It cleans up any remaining namespaces (unless they had test failures).
func CleanupNamespaces() {
	globalNSManager.mu.Lock()
	defer globalNSManager.mu.Unlock()

	// Clean up any remaining namespaces
	for containerPath, ns := range globalNSManager.containerToNS {
		if globalNSManager.containerFailed[containerPath] {
			log.Printf("Container '%s' had failures - preserving namespace '%s'", containerPath, ns)
		} else {
			log.Printf("Cleaning up namespace '%s' from container '%s'", ns, containerPath)
			oc.DeleteProjectIgnoreErrors(ns)
		}
	}
}

// AutoNamespacePerDescribe automatically creates a unique namespace for each
// Ordered Describe block and tracks it using container hierarchy.
//
// Usage in suite_test.go (called once per suite):
//
//	var testNS string
//	var _ = hooks.AutoNamespacePerDescribe(&testNS, func() *clients.Clients { return sharedClients })
//
//	var _ = AfterSuite(func() {
//		hooks.CleanupNamespaces() // Add this line to your existing AfterSuite
//		_ = config.RemoveTempDir()
//	})
//
// This will automatically:
// - Create a namespace when entering a new Ordered Describe block
// - Share that namespace across all It blocks in the Describe
// - Clean up the namespace when leaving the Describe (unless tests failed)
// - Update testNS variable for each test so diagnostics can access it.
//
// No need to add hooks to individual Describe blocks!
func AutoNamespacePerDescribe(namespacePtr *string, clientsFunc func() *clients.Clients) bool {
	manager := globalNSManager

	BeforeEach(func() {
		spec := CurrentSpecReport()

		// Build container path from hierarchy (exclude the It block itself)
		// Include NumAttempts so retries get a fresh namespace
		baseContainerPath := strings.Join(spec.ContainerHierarchyTexts, " > ")
		containerPath := baseContainerPath
		if spec.NumAttempts > 1 {
			// This is a retry - append attempt number to get unique key
			log.Printf("Retry detected: Attempt #%d for '%s'", spec.NumAttempts, baseContainerPath)
			containerPath = fmt.Sprintf("%s [attempt-%d]", baseContainerPath, spec.NumAttempts)
		}

		manager.mu.Lock()
		defer manager.mu.Unlock()

		// Detect if we've moved to a different container - clean up previous
		if manager.lastContainerPath != "" && manager.lastContainerPath != containerPath {
			log.Printf("Container switch detected: '%s' -> '%s'", manager.lastContainerPath, containerPath)

			// Clean up previous container's namespace if no failures
			if prevNS, exists := manager.containerToNS[manager.lastContainerPath]; exists {
				if manager.containerFailed[manager.lastContainerPath] {
					log.Printf("Previous container had failures - preserving namespace '%s'", prevNS)
				} else {
					log.Printf("Cleaning up namespace from previous container: %s", prevNS)
					oc.DeleteProjectIgnoreErrors(prevNS)
				}
				delete(manager.containerToNS, manager.lastContainerPath)
				delete(manager.containerFailed, manager.lastContainerPath)
			}
		}

		// If the container opts out of auto-namespace management (Label("no-auto-namespace")),
		// skip creation — the test manages its own static namespaces.
		optOut := false
		for _, clabels := range spec.ContainerHierarchyLabels {
			for _, l := range clabels {
				if l == "no-auto-namespace" {
					optOut = true
					break
				}
			}
			if optOut {
				break
			}
		}
		if optOut {
			manager.lastContainerPath = containerPath
			return
		}

		// Check if we're in a new container that doesn't have a namespace yet
		// For retries, containerPath includes attempt number, so this will be true
		if _, exists := manager.containerToNS[containerPath]; !exists {
			if spec.NumAttempts > 1 {
				log.Printf("AutoNamespacePerDescribe: Creating fresh namespace for retry attempt #%d", spec.NumAttempts)
			} else {
				log.Printf("AutoNamespacePerDescribe: New Describe block detected: %s", containerPath)
			}

			// Get clients
			cs := clientsFunc()
			if cs == nil {
				Fail("clients not initialized - ensure SynchronizedBeforeSuite has run")
			}

			// Generate unique namespace name
			ns := names.SimpleNameGenerator.RestrictLengthWithRandomSuffix("releasetest")
			manager.containerToNS[containerPath] = ns
			manager.containerFailed[containerPath] = false // initialize failure tracking

			log.Printf("Creating test namespace: %s for container: %s", ns, containerPath)
			oc.CreateNewProject(ns)

			// Wait for default service account to be created by operator
			// Kubernetes auto-creates "default" SA, but Tekton requires "pipeline" SA which is created by the operator
			_ = waitForPipelineServiceAccount(cs, ns, "pipeline")
		}

		// Update lastContainerPath for next iteration
		manager.lastContainerPath = containerPath

		// Update namespace pointer with current container's namespace
		if ns, ok := manager.containerToNS[containerPath]; ok {
			*namespacePtr = ns
			// Also store in store package so oc.Apply() etc can use it without explicit namespace
			store.SetNamespace(ns)
			// Re-scope all namespace-bound clients (PipelineRunClient, TaskRunClient, etc.)
			// to the current test namespace. sharedClients is initialized once in
			// SynchronizedBeforeSuite with config.TargetNamespace; without this update,
			// ValidatePipelineRun and similar helpers search in the wrong namespace.
			if cs := clientsFunc(); cs != nil {
				cs.NewClientSet(ns)
			}
		}
	})

	// Track failures per container
	AfterEach(func() {
		spec := CurrentSpecReport()
		if spec.Failed() {
			// Use same containerPath calculation as BeforeEach to match the namespace key
			baseContainerPath := strings.Join(spec.ContainerHierarchyTexts, " > ")
			containerPath := baseContainerPath
			if spec.NumAttempts > 1 {
				containerPath = fmt.Sprintf("%s [attempt-%d]", baseContainerPath, spec.NumAttempts)
			}

			manager.mu.Lock()
			manager.containerFailed[containerPath] = true
			manager.mu.Unlock()
			log.Printf("Test failed in container '%s' - namespace will be preserved", containerPath)
		}
	})

	return true
}

// waitForPipelineServiceAccount waits for the pipeline service account to be created
// The operator auto-creates this service account but it takes time
// Returns true if found, false if timeout (non-blocking - doesn't fail test)
func waitForPipelineServiceAccount(cs *clients.Clients, ns, targetSA string) bool {
	ctx := context.Background()
	err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, false, func(context.Context) (bool, error) {
		saList, err := cs.KubeClient.Kube.CoreV1().ServiceAccounts(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, sa := range saList.Items {
			if sa.Name == targetSA {
				log.Printf("Pipeline service account found in namespace %s", ns)
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		log.Printf("Warning: service account %s not found in namespace %s after timeout", targetSA, ns)
		return false
	}
	return true
}
