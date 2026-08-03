package triggers_test

import (
	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/triggers"
)

var oc = occmd.OC{}

var _ = Describe("Verify Triggers with cronjob: PIPELINES-04", Label("triggers", "local-test"), func() {
	It("Create Triggers using k8s cronJob: PIPELINES-04-TC01", Label("e2e", "triggers", "non-admin", "sanity", "sai"), func() {
		ns := lastNamespace
		DeferCleanup(func() { triggers.CleanupTriggers(sharedClients, "cron-listener", ns) })

		oc.Create("testdata/triggers/cron/example-pipeline.yaml", ns)
		oc.Create("testdata/triggers/cron/triggerbinding.yaml", ns)
		oc.Create("testdata/triggers/cron/triggertemplate.yaml", ns)
		oc.Create("testdata/triggers/cron/eventlistener.yaml", ns)

		routeURL := triggers.ExposeEventListener(sharedClients, "cron-listener", ns)

		cmd.MustSucceed("oc", "get", "is", "golang", "-n", "openshift")

		cronJobName := k8s.CreateCronJob(sharedClients, routeURL, "*/1 * * * *", ns)

		pipelines.WatchForPipelineRun(sharedClients, ns)

		k8s.DeleteCronJob(sharedClients, cronJobName, ns)

		pipelines.AssertForNoNewPipelineRunCreation(sharedClients, ns)
	})
})
