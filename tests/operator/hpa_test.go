package operator_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

var _ = Describe("Verify HPA: PIPELINES-13", Serial,
	Label("operator", "admin", "hpa"), func() {

		BeforeEach(func() {
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())
		})

		It("Test HPA for tekton-pipelines-webhook deployment: PIPELINES-13-TC01", func() {
			hpaList := cmd.MustSucceed("oc", "get", "hpa", "-n", "openshift-pipelines", "-o", "name").Stdout()
			Expect(hpaList).To(ContainSubstring("tekton-pipelines-webhook"),
				"HPA for tekton-pipelines-webhook not found in openshift-pipelines")
			Expect(hpaList).To(ContainSubstring("tekton-operator-proxy-webhook"),
				"HPA for tekton-operator-proxy-webhook not found in openshift-pipelines")
		})
	})
