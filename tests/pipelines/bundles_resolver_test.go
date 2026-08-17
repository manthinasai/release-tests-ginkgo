package pipelines_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Test bundles resolver functionality: PIPELINES-25-TC01", Label("e2e"), func() {
	var ns string

	BeforeEach(func() {
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should resolve tasks from bundles", func() {
		oc.Create("testdata/resolvers/pipelineruns/bundles-resolver-pipelinerun.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "bundles-resolver-pipelinerun", "successful", ns)
	})
})

var _ = Describe("Test bundles resolver with parameter: PIPELINES-25-TC02", Label("e2e", "sanity"), func() {
	var ns string

	BeforeEach(func() {
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should resolve tasks from bundles with parameters", func() {
		oc.Create("testdata/resolvers/pipelineruns/bundles-resolver-pipelinerun-param.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "bundles-resolver-pipelinerun-param", "successful", ns)
	})
})
