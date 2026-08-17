package triggers_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/triggers"
)

var _ = Describe("Verify triggerbindings spec: PIPELINES-10", Label("triggers"), func() {
	Describe("Verify CEL marshaljson function: PIPELINES-10-TC01", func() {
		It("Verify CEL marshaljson function", Label("e2e", "non-admin", "sanity"), func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/triggerbindings/cel-marshalJson.yaml", ns)
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "cel-marshaljson", ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, "cel-marshaljson", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, "cel-marshaljson", ns)
			pipelines.ValidateTaskRun(sharedClients, "cel-trig-marshaljson", "Success", ns)
		})
	})

	Describe("Verify event message body parsing with old annotation: PIPELINES-10-TC02", func() {
		It("Verify event message body parsing with old annotation", Label("e2e", "non-admin", "sanity"), func() {
			ns := lastNamespace
			oc.Create("testdata/triggers/triggerbindings/parse-json-body-with-annotation.yaml", ns)
			DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "parse-json-body-with-annotation", ns) })
			routeURL := triggers.ExposeEventListener(sharedClients, "parse-json-body-with-annotation", ns)
			resp, _ := triggers.MockPostEvent(routeURL, "github", "push", "testdata/push.json", false)
			triggers.AssertElResponse(sharedClients, resp, "parse-json-body-with-annotation", ns)
			pipelines.ValidateTaskRun(sharedClients, "trig-parse-json-body-with-annotation", "Success", ns)
		})
	})

	Describe("Verify event message body marshaling error: PIPELINES-10-TC03", func() {
		It("Verify event message body marshaling error", Label("non-admin"), Pending, func() {
			// Pending: tagged bug-to-fix in Gauge suite
		})
	})
})
