package olm_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
)

var _ = Describe("Console", Label("console"), func() {

	It("Verify icon on console", Label("manualonly"), func() {
		Skip("Manual verification only -- requires browser interaction with OpenShift console (login, navigate to OperatorHub, search for OpenShift Pipelines Operator, verify icon)")
	})
})
