package operator

import (
	"strings"
	"time"

	. "github.com/onsi/gomega" //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// AssertResourceCount polls until the expected number of resourceType exist in the current namespace.
func AssertResourceCount(resourceType string, expectedCount, timeoutSeconds int) {
	ns := store.Namespace()
	Eventually(func(g Gomega) {
		output := cmd.Run("oc", "get", resourceType, "-n", ns, "-o", "name").Stdout()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		count := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
		g.Expect(count).To(Equal(expectedCount),
			"expected %d %s(s) in namespace %s, got %d", expectedCount, resourceType, ns, count)
	}).WithTimeout(time.Duration(timeoutSeconds) * time.Second).WithPolling(config.APIRetry).Should(Succeed())
}

// CreatePrunerResources creates the 4 standard pruner test resources.
func CreatePrunerResources() {
	oc := occmd.OC{}
	ns := store.Namespace()
	oc.Create("testdata/pruner/pipeline/pipeline-for-pruner.yaml", ns)
	oc.Create("testdata/pruner/pipeline/pipelinerun-for-pruner.yaml", ns)
	oc.Create("testdata/pruner/task/task-for-pruner.yaml", ns)
	oc.Create("testdata/pruner/task/taskrun-for-pruner.yaml", ns)
}

// CreateAdditionalPrunerResources creates additional pipelinerun and taskrun resources.
func CreateAdditionalPrunerResources() {
	oc := occmd.OC{}
	ns := store.Namespace()
	oc.Create("testdata/pruner/pipeline/pipelinerun-for-pruner.yaml", ns)
	oc.Create("testdata/pruner/task/taskrun-for-pruner.yaml", ns)
}
