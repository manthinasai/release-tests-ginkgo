package hub_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
)

var oc = occmd.OC{}

var _ = Describe("Hub: PIPELINES-21", Serial, Label("hub"), func() {

	Describe("Install HUB without authentication: PIPELINES-21-TC01", Label("sanity"), Ordered, ContinueOnFailure, func() {

		It("creates TektonHub resource", func() {
			oc.Apply("testdata/hub/tektonhub.yaml", "")
		})

		It("verifies that the hub API deployment is up and running", func() {
			k8s.ValidateDeployments(sharedClients, config.TargetNamespace, config.HubAPIName)
		})

		It("verifies that the hub DB deployment is up and running", func() {
			k8s.ValidateDeployments(sharedClients, config.TargetNamespace, config.HubDBName)
		})

		It("verifies that the hub UI deployment is up and running", func() {
			k8s.ValidateDeployments(sharedClients, config.TargetNamespace, config.HubUIName)
		})

		It("searches for git-cli task via hub", func() {
			Expect(opc.HubSearch("git-cli")).To(Succeed())
		})
	})
})
