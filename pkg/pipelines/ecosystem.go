// Package pipelines provides helpers for creating and verifying Tekton Pipeline resources.
package pipelines

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/openshift"
)

// AssertTaskPresent verifies that a Task with the given name exists in the namespace.
func AssertTaskPresent(c *clients.Clients, namespace string, taskName string) {
	err := wait.PollUntilContextTimeout(c.Ctx, config.APIRetry, config.ResourceTimeout, false, func(context.Context) (bool, error) {
		log.Printf("Verifying if the task %v is present", taskName)
		_, err := c.Tekton.TektonV1().Tasks(namespace).Get(c.Ctx, taskName, v1.GetOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		Fail(fmt.Sprintf("tasks %v Expected: Present, Actual: Not Present, Error: %v", taskName, err))
	} else {
		log.Printf("Task %v is present", taskName)
	}
}

// AssertTaskNotPresent verifies that a Task with the given name does not exist in the namespace.
func AssertTaskNotPresent(c *clients.Clients, namespace string, taskName string) {
	err := wait.PollUntilContextTimeout(c.Ctx, config.APIRetry, config.ResourceTimeout, false, func(context.Context) (bool, error) {
		log.Printf("Verifying if the task %v is not present", taskName)
		_, err := c.Tekton.TektonV1().Tasks(namespace).Get(c.Ctx, taskName, v1.GetOptions{})
		if err == nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		Fail(fmt.Sprintf("tasks %v Expected: Not Present, Actual: Present, Error: %v", taskName, err))
	} else {
		log.Printf("Task %v is not present", taskName)
	}
}

// AssertStepActionPresent verifies that a StepAction with the given name exists in the namespace.
func AssertStepActionPresent(c *clients.Clients, namespace string, stepActionName string) {
	err := wait.PollUntilContextTimeout(c.Ctx, config.APIRetry, config.ResourceTimeout, false, func(context.Context) (bool, error) {
		log.Printf("Verifying if the stepAction %v is present", stepActionName)
		_, err := c.Tekton.TektonV1beta1().StepActions(namespace).Get(c.Ctx, stepActionName, v1.GetOptions{})
		if err == nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		Fail(fmt.Sprintf("StepAction %v Expected: Present, Actual: Not Present, Error: %v", stepActionName, err))
	} else {
		log.Printf("StepAction %v is present", stepActionName)
	}
}

// AssertStepActionNotPresent verifies that a StepAction with the given name does not exist in the namespace.
func AssertStepActionNotPresent(c *clients.Clients, namespace string, stepActionName string) {
	err := wait.PollUntilContextTimeout(c.Ctx, config.APIRetry, config.ResourceTimeout, false, func(context.Context) (bool, error) {
		log.Printf("Verifying if the stepAction %v is not present", stepActionName)
		_, err := c.Tekton.TektonV1beta1().StepActions(namespace).Get(c.Ctx, stepActionName, v1.GetOptions{})
		if err == nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		Fail(fmt.Sprintf("StepAction %v Expected: Not Present, Actual: Present, Error: %v", stepActionName, err))
	} else {
		log.Printf("StepAction %v is not present", stepActionName)
	}
}

// -----------------------------------------------------------------------
// S2I Pipeline Validation Helpers
//
// These helpers consolidate the common pattern of retrieving imagestream tags
// and running S2I pipelines for each tag (excluding "latest").
//
// Corresponds to Gauge step definitions in steps/cli/opc.go:
//   - "Start and verify pipeline ... with param ... with values stored in variable ..."
//   - "Start and verify dotnet pipeline ... with values stored in variable ..."
// -----------------------------------------------------------------------

// ParamCustomizer is a function that can customize params for a specific imagestream tag.
// It receives the tag value and the params map to modify.
//
// Example: DotnetParamCustomizer adds EXAMPLE_REVISION based on version number.
type ParamCustomizer func(tag string, params map[string]string)

// ValidateS2IPipelineForAllTags retrieves imagestream tags and runs the specified
// S2I pipeline for each tag (excluding "latest"), validating successful completion.
//
// This consolidates the Gauge pattern from steps/cli/opc.go:
//  1. Get tags of the imagestream "golang" from namespace "openshift" and store to variable "golang-tags"
//  2. Start and verify pipeline "s2i-go-pipeline" with param "VERSION" with values stored in variable "golang-tags"
//
// IMPORTANT: Pipelines are started concurrently using goroutines (matching Gauge behavior).
// All pipelines are started with a 3-second stagger between launches, then validation
// happens in parallel. This significantly reduces test execution time.
//
// Parameters:
//   - cs: Kubernetes/Tekton clients
//   - namespace: Target namespace for pipeline execution
//   - imagestreamName: Name of the OpenShift imagestream (e.g., "golang", "nodejs", "dotnet")
//   - pipelineName: Name of the pipeline to execute (e.g., "s2i-go-pipeline")
//   - customizer: Optional function to customize params per tag (e.g., DotnetParamCustomizer for dotnet)
//
// The function automatically:
//   - Retrieves all tags from the imagestream in "openshift" namespace
//   - Skips the "latest" tag
//   - Creates a workspace mapping to shared-pvc
//   - Sets VERSION param to the tag value
//   - Starts pipelines concurrently with 3-second stagger (matches Gauge)
//   - Validates all pipelineruns succeed in parallel
func ValidateS2IPipelineForAllTags(
	cs *clients.Clients,
	namespace string,
	imagestreamName string,
	pipelineName string,
	customizer ParamCustomizer,
) {
	tags := openshift.GetImageStreamTags(cs, "openshift", imagestreamName)
	if len(tags) == 0 {
		Fail(fmt.Sprintf("no tags found for imagestream %s", imagestreamName))
	}

	workspaces := map[string]string{"name=source": "claimName=shared-pvc"}
	var wg sync.WaitGroup // wait for all goroutines to finish

	for _, tag := range tags {
		if tag == "latest" {
			continue
		}

		// Create params for this tag
		params := map[string]string{"VERSION": tag}

		// Allow customization of params per tag (e.g., dotnet EXAMPLE_REVISION)
		if customizer != nil {
			customizer(tag, params)
		}

		log.Printf("Starting pipeline %s with VERSION=%s", pipelineName, tag)

		wg.Add(1)

		// Launch pipeline in goroutine to run concurrently
		go func(tag string, params map[string]string) {
			defer GinkgoRecover()
			defer wg.Done()
			customRunName := pipelineName + "-run-" + tag
			prName := opc.StartPipeline(pipelineName, params, workspaces, namespace,
				"--use-param-defaults", "--prefix-name", customRunName)
			ValidatePipelineRun(cs, prName, "successful", namespace)
		}(tag, params)

		// Brief stagger between pipeline starts to avoid API server thundering-herd.
		// 500ms is sufficient — the 3s inherited from Gauge was unnecessarily conservative.
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for all pipelines to complete
	wg.Wait()
}

// DotnetParamCustomizer adds the EXAMPLE_REVISION param for dotnet imagestreams
// based on version number:
//   - version >= 5.0: EXAMPLE_REVISION = "dotnet-X.Y"
//   - version <  5.0: EXAMPLE_REVISION = "dotnetcore-X.Y"
//
// This matches the Gauge dotnet step implementation in steps/cli/opc.go.
// Pass this customizer to ValidateS2IPipelineForAllTags for dotnet tests.
func DotnetParamCustomizer(tag string, params map[string]string) {
	re := regexp.MustCompile(`\d+\.\d+`)
	versionStr := re.FindString(tag)
	if versionStr != "" {
		versionFloat, _ := strconv.ParseFloat(versionStr, 64)
		if versionFloat >= 5.0 {
			params["EXAMPLE_REVISION"] = "dotnet-" + versionStr
		} else {
			params["EXAMPLE_REVISION"] = "dotnetcore-" + versionStr
		}
		log.Printf("Dotnet version %s: EXAMPLE_REVISION=%s", versionStr, params["EXAMPLE_REVISION"])
	}
}
