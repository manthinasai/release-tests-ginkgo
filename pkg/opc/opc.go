// Package opc provides wrappers around the opc CLI for pipeline and PAC operations.
package opc

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	"gotest.tools/v3/icmd"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
)

// Cmd holds the path to the opc binary for running opc CLI commands.
type Cmd struct {
	// path to opc binary
	Path string
}

// PipelineRunList holds the name and status of a PipelineRun.
type PipelineRunList struct {
	Name   string
	Status string
}

// PacInfoInstall holds installation info for Pipelines-as-Code.
type PacInfoInstall struct {
	PipelinesAsCode PipelinesAsCodeSection
}

// PipelinesAsCodeSection holds version and namespace info for the PipelinesAsCode installation.
type PipelinesAsCodeSection struct {
	InstallVersion   string
	InstallNamespace string
}

// New initializes Cmd
func New(opcPath string) Cmd {
	return Cmd{
		Path: opcPath,
	}
}

// GetOPCServerVersion returns the server-side version of the given Tekton component.
func GetOPCServerVersion(component string) string {
	var version string
	output := cmd.MustSucceed("opc", "version", "-s").Stdout()
	titleComp := strings.ToUpper(component[:1]) + component[1:]
	prefix := titleComp + " version:"

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			version = strings.TrimSpace(strings.TrimPrefix(line, prefix))
			break
		}
	}

	if strings.Contains(version, "unknown") {
		Fail(fmt.Sprintf("%s is not installed", titleComp))
	}
	return version
}

// AssertComponentVersion verifies the installed version of the given OpenShift Pipelines component.
func AssertComponentVersion(version string, component string) {
	var actualVersion string
	switch component {
	case "pipeline", "triggers", "operator", "chains":
		actualVersion = GetOPCServerVersion(component)
	case "OSP":
		actualVersion = cmd.MustSucceed("oc", "get", "tektonconfig", "config", "-o", "jsonpath={.status.version}").Stdout()
	case "pac":
		actualVersion = cmd.MustSucceed("oc", "get", "pac", "pipelines-as-code", "-o", "jsonpath={.status.version}").Stdout()
	case "hub":
		actualVersion = cmd.MustSucceed("oc", "get", "tektonhub", "hub", "-o", "jsonpath={.status.version}").Stdout()
	case "results":
		actualVersion = cmd.MustSucceed("oc", "get", "tektonresult", "result", "-o", "jsonpath={.status.version}").Stdout()
	case "manual-approval-gate":
		actualVersion = cmd.MustSucceed("oc", "get", "manualapprovalgate", "manual-approval-gate", "-o", "jsonpath={.status.version}").Stdout()
	case "pruner":
		actualVersion = cmd.MustSucceed("oc", "get", "tektonpruner", "pruner", "-o", "jsonpath={.status.version}").Stdout()
	default:
		Fail(fmt.Sprintln("Unknown component"))
	}

	actualVersion = strings.Trim(actualVersion, "\n")
	if !strings.Contains(actualVersion, version) {
		Fail(fmt.Sprintf("The %s has an unexpected version: %s, expected: %s", component, actualVersion, version))
	}
}

// DownloadCLIFromCluster downloads the tkn CLI binary from the cluster's console download URL.
func DownloadCLIFromCluster() {
	var architecture = strings.Trim(cmd.MustSucceed("uname").Stdout(), "\n") + " " + strings.Trim(cmd.MustSucceed("uname", "-m").Stdout(), "\n")
	var cliDownloadURL = cmd.MustSucceed("oc", "get", "consoleclidownloads", "tkn", "-o", "jsonpath={.spec.links[?(@.text==\"Download tkn and tkn-pac for "+architecture+"\")].href}").Stdout()
	cmd.MustSucceedIncreasedTimeout(time.Minute*10, "curl", "-o", "/tmp/tkn-binary.tar.gz", "-k", cliDownloadURL)
	cmd.MustSucceed("tar", "-xf", "/tmp/tkn-binary.tar.gz", "-C", "/tmp")
}

// AssertClientVersion verifies that the client-side version of the given binary matches the expected version.
func AssertClientVersion(binary string) {
	var commandResult, unexpectedVersion string

	switch binary {
	case "tkn-pac":
		commandResult = cmd.MustSucceed("/tmp/tkn-pac", "version").Stdout()
		expectedVersion := os.Getenv("PAC_VERSION")
		if !strings.Contains(commandResult, expectedVersion) {
			Fail(fmt.Sprintf("tkn-pac has an unexpected version: %s. Expected: %s", commandResult, expectedVersion))
		}

	case "tkn":
		expectedVersion := os.Getenv("TKN_CLIENT_VERSION")
		commandResult = cmd.MustSucceed("/tmp/tkn", "version").Stdout()
		var splittedCommandResult = strings.Split(commandResult, "\n")
		for i := range splittedCommandResult {
			if strings.Contains(splittedCommandResult[i], "Client") {
				if !strings.Contains(splittedCommandResult[i], expectedVersion) {
					unexpectedVersion = splittedCommandResult[i]
					Fail(fmt.Sprintf("tkn client has an unexpected version: %s. Expected: %s", unexpectedVersion, expectedVersion))
				}
			}
		}

	case "opc":
		commandResult = cmd.MustSucceed("/tmp/opc", "version").Stdout()
		components := [3]string{"OpenShift Pipelines Client", "Tekton CLI", "Pipelines as Code CLI"}
		expectedVersions := [3]string{os.Getenv("OSP_VERSION"), os.Getenv("TKN_CLIENT_VERSION"), os.Getenv("PAC_VERSION")}
		splittedCommandResult := strings.Split(commandResult, "\n")
		for i := 0; i < 3; i++ {
			if strings.Contains(splittedCommandResult[i], components[i]) {
				if !strings.Contains(splittedCommandResult[i], expectedVersions[i]) {
					unexpectedVersion = splittedCommandResult[i]
					Fail(fmt.Sprintf("%s has an unexpected version: %s. Expected: %s", components[i], unexpectedVersion, expectedVersions[i]))
				}
			}
		}

	default:
		Fail("unknown binary or client")
	}
}

// AssertServerVersion verifies that the server-side component versions match the expected versions.
func AssertServerVersion(binary string) {
	var commandResult, unexpectedVersion string

	switch binary {
	case "opc":
		commandResult = cmd.MustSucceed("/tmp/opc", "version", "--server").Stdout()
		components := [4]string{"Chains version", "Pipeline version", "Triggers version", "Operator version"}
		expectedVersions := [4]string{os.Getenv("CHAINS_VERSION"), os.Getenv("PIPELINE_VERSION"), os.Getenv("TRIGGERS_VERSION"), os.Getenv("OPERATOR_VERSION")}
		splittedCommandResult := strings.Split(commandResult, "\n")
		for i := 0; i < 4; i++ {
			if strings.Contains(splittedCommandResult[i], components[i]) {
				if !strings.Contains(splittedCommandResult[i], expectedVersions[i]) {
					unexpectedVersion = splittedCommandResult[i]
					Fail(fmt.Sprintf("%s has an unexpected version: %s. Expected: %s", components[i], unexpectedVersion, expectedVersions[i]))
				}
			}
		}
	default:
		Fail("unknown binary or client")
	}

}

// ValidateQuickstarts verifies that the expected console quickstart resources exist.
func ValidateQuickstarts() {
	cmd.MustSucceed("oc", "get", "consolequickstart", "install-app-and-associate-pipeline").Stdout()
	cmd.MustSucceed("oc", "get", "consolequickstart", "configure-pipeline-metrics").Stdout()
}

// MustSucceed runs opc with the given arguments and fails the test on non-zero exit.
func (opc Cmd) MustSucceed(args ...string) string {
	return opc.Assert(icmd.Success, args...)
}

// Assert runs opc with the given arguments and asserts the expected result.
func (opc Cmd) Assert(exp icmd.Expected, args ...string) string {
	run := append([]string{opc.Path}, args...)
	output := cmd.Assert(exp, run...)
	return output.Stdout()
}

// StartPipeline starts the given pipeline with the specified params, workspaces, and extra args.
func StartPipeline(pipelineName string, params map[string]string, workspaces map[string]string, namespace string, args ...string) string {
	commandArgs := make([]string, 0, 8+len(params)+len(workspaces)+len(args))
	commandArgs = append(commandArgs, "opc", "pipeline", "start", pipelineName, "-o", "name", "-n", namespace)
	for key, value := range params {
		commandArgs = append(commandArgs, fmt.Sprintf("-p %s=%s", key, value))
	}
	for key, value := range workspaces {
		commandArgs = append(commandArgs, fmt.Sprintf("-w %s,%s", key, value))
	}
	commandArgs = append(commandArgs, args...)
	// Build args correctly without join+split (which breaks args containing spaces)
	flatArgs := make([]string, 0, len(commandArgs))
	for _, arg := range commandArgs {
		flatArgs = append(flatArgs, strings.Fields(arg)...)
	}
	pipelineRunName := strings.Trim(cmd.MustSucceed(flatArgs...).Stdout(), "\n")
	log.Printf("Pipelinerun %s started", pipelineRunName)
	return pipelineRunName
}

// GetOpcPacInfoInstall fetches Pipelines as Code install information
func GetOpcPacInfoInstall() (*PacInfoInstall, error) {
	output := cmd.MustSucceed("opc", "pac", "info", "install").Stdout()
	lines := strings.Split(output, "\n")

	var pacInfo PacInfoInstall
	section := "" // current section: "pipelines"

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if line == "Pipelines as Code:" {
			section = "pipelines"
			continue
		}
		if section == "pipelines" {
			if strings.HasPrefix(line, "Install Version:") {
				pacInfo.PipelinesAsCode.InstallVersion = strings.TrimSpace(strings.TrimPrefix(line, "Install Version:"))
			} else if strings.HasPrefix(line, "Install Namespace:") {
				pacInfo.PipelinesAsCode.InstallNamespace = strings.TrimSpace(strings.TrimPrefix(line, "Install Namespace:"))
			}
		}
	}

	// Verify install version is not empty
	if pacInfo.PipelinesAsCode.InstallVersion == "" {
		return nil, fmt.Errorf("output of 'opc pac info install' is empty or missing Pipelines as Code information")
	}

	return &pacInfo, nil
}

// HubSearch performs an opc hub search for a resource
func HubSearch(resource string) error {
	output := cmd.MustSucceed("opc", "hub", "search", resource).Stdout()

	if !strings.Contains(output, resource) {
		log.Printf("Resource %q not found in opc hub search", resource)
		return fmt.Errorf("hub search failed for %s", resource)
	}
	return nil
}

// GetOpcPrList fetches pipeline run lists with status of each run
func GetOpcPrList(pipelineRunName, namespace string) ([]PipelineRunList, error) {
	result, err := VerifyResourceListMatchesName("pipelinerun", pipelineRunName, namespace)
	if err != nil {
		Fail(fmt.Sprintf("Failed to get pipelinerun list: %v", err))
	}
	output := strings.TrimSpace(result)
	lines := strings.Split(output, "\n")

	// Ensure output isn't empty
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected pipelinerun output %s", output)
	}

	var runs []PipelineRunList
	for _, line := range lines[1:] { // Skip header
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			log.Printf("Skipping malformed row: %s", line)
			continue
		}

		run := PipelineRunList{
			Name:   fields[0],
			Status: fields[len(fields)-1],
		}
		runs = append(runs, run)
	}

	return runs, nil
}

// resourceExists checks if a resource exists in output
func resourceExists(output, resourceName string) bool {
	trimmedOutput := strings.TrimSpace(output)
	if strings.HasPrefix(trimmedOutput, "No") {
		return false
	}

	lines := strings.Split(trimmedOutput, "\n")
	resourceLines := lines[1:] // Skip header line

	for _, line := range resourceLines {
		// Trim spaces from each line
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue // Skip empty lines
		}
		// Split the line into fields
		fields := strings.Fields(trimmed)
		if len(fields) > 0 && fields[0] == resourceName {
			return true
		}
	}
	return false
}

// VerifyResourceListMatchesName verifies that the named resource appears in the opc resource list for the given namespace.
func VerifyResourceListMatchesName(resourceType, name, namespace string) (string, error) {
	output := cmd.MustSucceed("opc", resourceType, "list", "-n", namespace).Stdout()
	if !resourceExists(output, name) {
		return "", fmt.Errorf("%s %q not found in namespace %q", resourceType, name, namespace)
	}
	return output, nil
}
