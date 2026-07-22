package operator_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/operator"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// PIPELINES-11
var _ = Describe("Verify RBAC Resources and CA Bundle Configuration", Serial, Ordered, ContinueOnFailure,
	Label("e2e", "operator", "admin", "rbac"), func() {

		BeforeAll(func() {
			lastNamespace = config.TargetNamespace
			operator.ValidateOperatorInstallStatus(sharedClients, store.GetCRNames())

			DeferCleanup(func() {
				oc.UpdateTektonConfigParam("createRbacResource", "true")
				oc.UpdateTektonConfigParam("createCABundleConfigMaps", "true")
			})
		})

		// PIPELINES-11-TC01
		It("Disable RBAC resource creation", Label("sanity", "rbac-disable"), func() {
			oc.UpdateTektonConfigParam("createRbacResource", "true")
			operator.ValidateRBAC(sharedClients, store.GetCRNames())

			oc.UpdateTektonConfigParam("createRbacResource", "false")
			operator.ValidateRBACAfterDisable(sharedClients, store.GetCRNames())

			oc.UpdateTektonConfigParam("createRbacResource", "true")
			operator.ValidateRBAC(sharedClients, store.GetCRNames())
		})

		// PIPELINES-11-TC02
		It("Independent CA Bundle ConfigMap creation control", Label("sanity", "cabundle-control"), func() {
			oc.UpdateTektonConfigParam("createCABundleConfigMaps", "true")
			operator.ValidateCABundleConfigMaps(sharedClients, store.GetCRNames())

			oc.UpdateTektonConfigParam("createCABundleConfigMaps", "false")
			operator.ValidateCABundleConfigMaps(sharedClients, store.GetCRNames())
		})
	})
