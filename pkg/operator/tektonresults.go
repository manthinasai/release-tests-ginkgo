package operator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/store"
)

// CreateSecretsForTektonResults creates the required secrets for Tekton Results.
func CreateSecretsForTektonResults() {
	var password = cmd.MustSucceed("openssl", "rand", "-base64", "20").Stdout()
	password = strings.ReplaceAll(password, "\n", "")
	cmd.MustSucceed("oc", "create", "secret", "-n", "openshift-pipelines", "generic", "tekton-results-postgres", "--from-literal=POSTGRES_USER=result", "--from-literal=POSTGRES_PASSWORD="+password)
	// generating tls certificate
	cmd.MustSucceed("openssl", "req", "-x509", "-newkey", "rsa:4096", "-keyout", "key.pem", "-out", "cert.pem", "-days", "365", "-nodes", "-subj", "/CN=tekton-results-api-service.openshift-pipelines.svc.cluster.local", "-addext", "subjectAltName=DNS:tekton-results-api-service.openshift-pipelines.svc.cluster.local")
	// creating secret with generated certificate
	cmd.MustSucceed("oc", "create", "secret", "tls", "-n", "openshift-pipelines", "tekton-results-tls", "--cert=cert.pem", "--key=key.pem")
}

// EnsureResultsReady waits until the TektonResults deployment is ready.
func EnsureResultsReady() {
	cmd.MustSucceedIncreasedTimeout(time.Minute*5, "oc", "wait", "--for=condition=Ready", "tektoninstallerset", "-l", "operator.tekton.dev/type=result", "--timeout=120s")
}

// CreateResultsRoute creates the OpenShift route for the Tekton Results API.
func CreateResultsRoute() {
	cmd.Run("oc", "create", "route", "-n", "openshift-pipelines", "passthrough", "tekton-results-api-service", "--service=tekton-results-api-service", "--port=8080")
}

// GetResultsAPI returns the Tekton Results API endpoint as an https:// URL, the
// form expected by "opc results config set --host".
func GetResultsAPI() string {
	host := cmd.MustSucceed("oc", "get", "route", "tekton-results-api-service", "-n", "openshift-pipelines", "--no-headers", "-o", "custom-columns=:spec.host").Stdout()
	return "https://" + strings.TrimSpace(host)
}

// configureResultsCLIOnce guards the kubeconfig write performed by
// "opc results config set" so it happens at most once per test process.
var configureResultsCLIOnce sync.Once

// ConfigureResultsCLI points the Results CLI at the cluster's Results API route.
// The connection settings are stored in the kubeconfig rather than passed per
// invocation, which is why this replaces the removed --addr and --insecure flags.
func ConfigureResultsCLI() {
	configureResultsCLIOnce.Do(func() {
		args := []string{"opc", "results", "config", "set",
			"--host=" + GetResultsAPI(),
			"--insecure-skip-tls-verify",
		}
		// Token-authenticated users need an explicit bearer token. Certificate-based
		// logins (for example system:admin) have none and authenticate via the
		// kubeconfig, so only pass --token when one is actually available.
		if token := strings.TrimSpace(cmd.Run("oc", "whoami", "-t").Stdout()); token != "" {
			args = append(args, "--token="+token)
		}
		if config.Flags.Kubeconfig != "" {
			args = append(args, "--kubeconfig="+config.Flags.Kubeconfig)
		}
		cmd.MustSucceed(args...)
	})
}

// lastRunName returns the name of the most recent run of the given type in ns.
// The Results commands address runs by name, where the removed ones took the
// record UUID from the results.tekton.dev/record annotation.
func lastRunName(resourceType, ns string) string {
	name := cmd.MustSucceed("tkn", resourceType, "describe", "--last", "-o", "jsonpath={.metadata.name}", "-n", ns).Stdout()
	return strings.Trim(strings.TrimSpace(name), "'")
}

// GetResultsAnnotations returns the results name, record UUID, and log URL annotations for the given resource.
func GetResultsAnnotations(resourceType string) (string, string, string) {
	ns := store.Namespace()
	if ns == "" {
		panic("GetResultsAnnotations: store.Namespace() is empty - ensure hooks are configured")
	}
	var resultUUID = cmd.MustSucceed("opc", resourceType, "describe", "--last", "-o", "jsonpath='{.metadata.annotations.results\\.tekton\\.dev/result}'", "-n", ns).Stdout()
	var recordUUID = cmd.MustSucceed("opc", resourceType, "describe", "--last", "-o", "jsonpath='{.metadata.annotations.results\\.tekton\\.dev/record}'", "-n", ns).Stdout()
	var stored = cmd.MustSucceed("opc", resourceType, "describe", "--last", "-o", "jsonpath='{.metadata.annotations.results\\.tekton\\.dev/stored}'", "-n", ns).Stdout()
	recordUUID = strings.ReplaceAll(recordUUID, "'", "")
	resultUUID = strings.ReplaceAll(resultUUID, "'", "")
	stored = strings.ReplaceAll(stored, "'", "")
	return resultUUID, recordUUID, stored
}

func getRunsAnnotations(cs *clients.Clients, resourceType, name string) (map[string]string, error) {
	switch resourceType {
	case "taskrun":
		taskRun, err := cs.TaskRunClient.Get(cs.Ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return taskRun.GetAnnotations(), nil
	case "pipelinerun":
		pipelineRuns, err := cs.PipelineRunClient.Get(cs.Ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return pipelineRuns.GetAnnotations(), nil
	default:
		return nil, fmt.Errorf("invalid resource type: %s", resourceType)
	}
}

// VerifyResultsAnnotationStored verifies that Results annotations are stored on the resource.
func VerifyResultsAnnotationStored(cs *clients.Clients, resourceType string) error {
	ns := store.Namespace()
	if ns == "" {
		return fmt.Errorf("VerifyResultsAnnotationStored: store.Namespace() is empty - ensure hooks are configured")
	}
	resourceName := cmd.MustSucceed("tkn", resourceType, "describe", "--last", "-o", "jsonpath='{.metadata.name}'", "-n", ns).Stdout()
	resourceName = strings.ReplaceAll(resourceName, "'", "")

	log.Printf("Waiting for annotation 'results.tekton.dev/stored' to be true \n")
	err := wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, true, func(context.Context) (done bool, err error) {
		annotations, err := getRunsAnnotations(cs, resourceType, resourceName)
		if err != nil {
			return false, err
		}
		if annotations == nil || annotations["results.tekton.dev/stored"] == "" {
			log.Printf("Annotation 'results.tekton.dev/stored' is not set yet\n")
			return false, nil
		}
		if annotations["results.tekton.dev/stored"] == "true" {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		return fmt.Errorf("annotation 'results.tekton.dev/stored' is not true: %w", err)
	}
	return nil
}

// VerifyResultsLogs verifies that Results logs are available for the given resource type.
func VerifyResultsLogs(resourceType string) error {
	ns := store.Namespace()
	if ns == "" {
		return fmt.Errorf("VerifyResultsLogs: store.Namespace() is empty - ensure hooks are configured")
	}

	_, recordUUID, _ := GetResultsAnnotations(resourceType)
	if recordUUID == "" {
		return fmt.Errorf("annotation results.tekton.dev/record is not set")
	}

	// Wait for Results API to finish indexing after annotation is set
	// The annotation=true means data was sent, but API needs time to index it
	log.Printf("Waiting 10 seconds for Results API to index data\n")
	time.Sleep(10 * time.Second)

	ConfigureResultsCLI()
	// Unlike the removed "results logs get", this returns the log text directly
	// rather than a JSON envelope with base64-encoded data.
	resultsLogs := cmd.MustSucceed("opc", "results", resourceType, "logs", lastRunName(resourceType, ns), "-n", ns).Stdout()
	if strings.Contains(resultsLogs, "record not found") {
		return fmt.Errorf("results log not found")
	}
	if !strings.Contains(resultsLogs, "Hello, Results!") || !strings.Contains(resultsLogs, "Goodbye, Results!") {
		return fmt.Errorf("logs are incorrect: expected 'Hello, Results!' and 'Goodbye, Results!'")
	}
	return nil
}

// VerifyResultsRecords verifies that the expected result records exist via the Results API.
func VerifyResultsRecords(resourceType string) error {
	ns := store.Namespace()
	if ns == "" {
		return fmt.Errorf("VerifyResultsRecords: store.Namespace() is empty - ensure hooks are configured")
	}

	// Wait for Results API to finish indexing after annotation is set
	// The annotation=true means data was sent, but API needs time to index it
	log.Printf("Waiting 10 seconds for Results API to index data\n")
	time.Sleep(10 * time.Second)

	ConfigureResultsCLI()
	// Unlike the removed "results records get", this returns the stored run object
	// itself rather than a record envelope with a base64-encoded value.
	resultsRecord := cmd.MustSucceed("opc", "results", resourceType, "describe", lastRunName(resourceType, ns), "-n", ns, "-o", "json").Stdout()
	if strings.Contains(resultsRecord, "record not found") {
		return fmt.Errorf("results record not found")
	}
	if !strings.Contains(resultsRecord, "Hello, Results!") || !strings.Contains(resultsRecord, "Goodbye, Results!") {
		return fmt.Errorf("records are incorrect: expected 'Hello, Results!' and 'Goodbye, Results!'")
	}
	return nil
}
