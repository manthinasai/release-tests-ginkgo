package pipelines_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Run Pipeline with non-existent ServiceAccount: PIPELINES-02-TC01", Label("e2e", "negative", "non-admin", "sanity"), func() {
	var ns string

	BeforeEach(func() {
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should fail when ServiceAccount does not exist", func() {
		// Verify ServiceAccount "foobar" does not exist
		result := cmd.Run("oc", "get", "serviceaccount", "foobar", "-n", ns)
		Expect(result.ExitCode).NotTo(Equal(0), "ServiceAccount 'foobar' should not exist")

		oc.Create("testdata/negative/v1beta1/pipelinerun.yaml", ns)
		pipelines.ValidatePipelineRun(sharedClients, "output-pipeline-run-vb", "Failure", ns)
	})
})

var _ = Describe("Run Task with non-existent ServiceAccount: PIPELINES-02-TC02", Label("e2e", "negative", "non-admin"), func() {
	var ns string

	BeforeEach(func() {
		ns = store.Namespace()
		sharedClients.NewClientSet(ns)
	})

	It("should fail when ServiceAccount does not exist", func() {
		// Verify ServiceAccount "foobar" does not exist
		result := cmd.Run("oc", "get", "serviceaccount", "foobar", "-n", ns)
		Expect(result.ExitCode).NotTo(Equal(0), "ServiceAccount 'foobar' should not exist")

		oc.Create("testdata/negative/v1beta1/pull-request.yaml", ns)
		pipelines.ValidateTaskRun(sharedClients, "pullrequest-vb", "Failure", ns)
	})
})
