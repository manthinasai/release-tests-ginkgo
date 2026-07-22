package ecosystem_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // dot import is idiomatic for Gomega

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/triggers"
)

// PIPELINES-33
// -----------------------------------------------------------------------
// S2I Ecosystem Tests
//
// Source-to-Image (S2I) tests verify that S2I builder images can compile
// and deploy applications.
//
// TC01 is unique: it creates a full pipeline + deployment + route and
// validates the HTTP response.
//
// TC02-TC09 use the "imagestream-start" pattern via ValidateS2IPipelineForAllTags
// helper function (see helpers.go).
// -----------------------------------------------------------------------

// PIPELINES-33-TC01
var _ = Describe("S2I nodejs pipelinerun with route validation", Label("ecosystem", "e2e", "s2i", "sanity"), func() {

	It("should create nodejs pipeline, verify pipelinerun, expose route and validate response", func() {
		ns := createTestNamespace("eco-s2i-nodejs-route")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		// Create all resources
		oc.Create("testdata/ecosystem/pipelines/nodejs-ex-git.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)
		oc.Create("testdata/ecosystem/deploymentconfigs/nodejs-ex-git.yaml", ns)
		oc.Create("testdata/ecosystem/imagestreams/nodejs-ex-git.yaml", ns)
		oc.Create("testdata/ecosystem/pipelineruns/nodejs-ex-git.yaml", ns)

		// Verify pipelinerun
		pipelines.ValidatePipelineRun(sharedClients, "nodejs-ex-git-pr", "successful", ns)

		// Expose deployment config on port 3000
		triggers.ExposeDeploymentConfig("nodejs-ex-git", "3000", ns)

		// Get route URL
		routeURL := triggers.GetRouteURL("nodejs-ex-git", ns)

		// Validate route response contains expected content
		output := cmd.MustSucceedIncreasedTimeout(180*time.Second, "curl", "-kL", routeURL).Stdout()
		Expect(output).To(ContainSubstring("See Also"))
	})
})

// PIPELINES-33-TC02
var _ = Describe("S2I dotnet pipelinerun", Label("ecosystem", "e2e", "s2i"), func() {

	It("should create dotnet pipeline and verify pipelinerun for each imagestream tag", func() {
		// Skip on ppc64le architecture
		if config.Flags.ClusterArch == "ppc64le" {
			Skip(fmt.Sprintf("test skipped on architecture: %s", config.Flags.ClusterArch))
		}

		ns := createTestNamespace("eco-s2i-dotnet")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		// Create pipeline and PVC resources
		oc.Create("testdata/ecosystem/pipelines/s2i-dotnet.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		// Validate all imagestream tags using helper with dotnet-specific customizer
		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "dotnet", "s2i-dotnet-pipeline", pipelines.DotnetParamCustomizer)
	})
})

// PIPELINES-33-TC03
var _ = Describe("S2I golang pipelinerun", Label("ecosystem", "e2e", "sanity", "s2i"), func() {

	It("should create golang pipeline and verify pipelinerun for each imagestream tag", func() {
		ns := createTestNamespace("eco-s2i-golang")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/s2i-go.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "golang", "s2i-go-pipeline", nil)
	})
})

// PIPELINES-33-TC04
var _ = Describe("S2I java pipelinerun", Label("ecosystem", "e2e", "s2i"), func() {

	It("should create java pipeline and verify pipelinerun for each imagestream tag", func() {
		ns := createTestNamespace("eco-s2i-java")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/s2i-java.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "java", "s2i-java-pipeline", nil)
	})
})

// PIPELINES-33-TC05
var _ = Describe("S2I nodejs pipelinerun", Label("ecosystem", "e2e", "s2i"), func() {

	It("should create nodejs pipeline and verify pipelinerun for each imagestream tag", func() {
		ns := createTestNamespace("eco-s2i-nodejs")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/s2i-nodejs.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "nodejs", "s2i-nodejs-pipeline", nil)
	})
})

// PIPELINES-33-TC06
var _ = Describe("S2I perl pipelinerun", Label("ecosystem", "e2e", "s2i"), func() {

	It("should create perl pipeline and verify pipelinerun for each imagestream tag", func() {
		ns := createTestNamespace("eco-s2i-perl")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/s2i-perl.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "perl", "s2i-perl-pipeline", nil)
	})
})

// PIPELINES-33-TC07
var _ = Describe("S2I php pipelinerun", Label("ecosystem", "e2e", "s2i"), func() {

	It("should create php pipeline and verify pipelinerun for each imagestream tag", func() {
		ns := createTestNamespace("eco-s2i-php")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/s2i-php.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "php", "s2i-php-pipeline", nil)
	})
})

// PIPELINES-33-TC08
var _ = Describe("S2I python pipelinerun", Label("ecosystem", "e2e", "s2i"), func() {

	It("should create python pipeline and verify pipelinerun for each imagestream tag", func() {
		ns := createTestNamespace("eco-s2i-python")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/s2i-python.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "python", "s2i-python-pipeline", nil)
	})
})

// PIPELINES-33-TC09
var _ = Describe("S2I ruby pipelinerun", Label("ecosystem", "e2e", "s2i"), func() {

	It("should create ruby pipeline and verify pipelinerun for each imagestream tag", func() {
		ns := createTestNamespace("eco-s2i-ruby")
		lastNamespace = ns
		DeferCleanup(oc.DeleteProjectIgnoreErrors, ns)
		sharedClients.NewClientSet(ns)

		oc.Create("testdata/ecosystem/pipelines/s2i-ruby.yaml", ns)
		oc.Create("testdata/pvc/pvc.yaml", ns)

		pipelines.ValidateS2IPipelineForAllTags(sharedClients, ns, "ruby", "s2i-ruby-pipeline", nil)
	})
})
