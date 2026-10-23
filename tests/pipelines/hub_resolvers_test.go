package pipelines_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// Hub resolvers spec had no Polarion test case ID in the original Gauge spec.
var _ = Describe("Test hub resolver functionality: PIPELINES-17", Label("e2e", "sanity"), func() {
	var ns string

	BeforeEach(func() {
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should resolve tasks from Tekton Hub: PIPELINES-17-TC01", func() {
		oc.Apply("testdata/resolvers/pipelines/git-cli-hub.yaml", ns)
		oc.Apply("testdata/pvc/pvc.yaml", ns)
		oc.Apply("testdata/resolvers/pipelineruns/git-cli-hub.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "hub-git-cli-run", "successful", ns)
	})
})
