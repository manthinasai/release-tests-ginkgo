// Package monitoring provides helpers for querying Prometheus metrics in integration tests.
package monitoring

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"

	v1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	prom "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prommodel "github.com/prometheus/common/model"
)

type authRoundtripper struct {
	authorization string
	inner         http.RoundTripper
}

// RoundTrip executes the HTTP request with an Authorization bearer token injected.
func (a *authRoundtripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", a.authorization)
	return a.inner.RoundTrip(r)
}

func newPrometheusClient(cs *clients.Clients) (promv1.API, error) {
	route, err := getPrometheusRoute(cs)
	if err != nil {
		return nil, err
	}
	bToken, err := getBearerTokenForPrometheusAccount(cs)
	if err != nil {
		return nil, err
	}

	rt := prom.DefaultRoundTripper.(*http.Transport).Clone()
	// nolint reason: InsecureSkipVerify is enabled due to self signed certs
	rt.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	client, err := prom.NewClient(prom.Config{
		Address: "https://" + route.Spec.Host,
		RoundTripper: &authRoundtripper{
			authorization: fmt.Sprintf("Bearer %s", bToken),
			inner:         rt,
		},
	})
	if err != nil {
		return nil, err
	}

	return promv1.NewAPI(client), nil
}

func getPrometheusRoute(cs *clients.Clients) (*v1.Route, error) {
	r, err := cs.Route.Routes("openshift-monitoring").Get(context.Background(), "prometheus-k8s", meta.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting Prometheus route: %w", err)
	}
	return r, nil
}

// TargetService holds the Prometheus job name and expected value for a metrics assertion.
type TargetService struct {
	Job           string
	ExpectedValue string
}

// VerifyHealthStatusMetric verifies that the health status metric for the given service is up.
func VerifyHealthStatusMetric(cs *clients.Clients, targetService TargetService) error {
	pc, err := newPrometheusClient(cs)
	if err != nil {
		return err
	}
	if err := wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, true, func(context.Context) (bool, error) {
		value, _, err := pc.Query(context.Background(), fmt.Sprintf(`max(up{job="%s"})`, targetService.Job), time.Time{})
		if err != nil {
			return false, err
		}

		vec, ok := value.(prommodel.Vector)
		if !ok {
			return false, nil
		}

		if len(vec) < 1 {
			return false, nil
		}
		log.Printf("Target Job: [%v] ready/up status, Actual: [%v], Expected: [%+v]", targetService.Job, vec[0].Value.String(), targetService.ExpectedValue)
		return vec[0].Value.String() == targetService.ExpectedValue, nil
	}); err != nil {
		return fmt.Errorf("failed to access the Prometheus API endpoint and get the metric value expected: %w", err)
	}

	return nil
}

// VerifyPipelinesControlPlaneMetrics verifies that the pipelines controller is emitting metrics.
// Only checks metrics that are always present from controller startup, without requiring
// any PipelineRun or TaskRun activity. Activity-dependent metrics (counters, histograms)
// are tested separately in the OTel migration test suite.
func VerifyPipelinesControlPlaneMetrics(cs *clients.Clients) error {
	pipelineMetrics := []string{
		"tekton_pipelines_controller_running_pipelineruns",
		"tekton_pipelines_controller_running_taskruns",
		"tekton_pipelines_controller_running_pipelineruns_waiting_on_pipeline_resolution",
		"tekton_pipelines_controller_running_pipelineruns_waiting_on_task_resolution",
	}
	for _, metric := range pipelineMetrics {
		if err := VerifyMetricExists(cs, metric); err != nil {
			return err
		}
	}
	return nil
}

// VerifyMetricExists verifies that a metric is present in Prometheus.
func VerifyMetricExists(cs *clients.Clients, metricName string) error {
	pc, err := newPrometheusClient(cs)
	if err != nil {
		return err
	}
	if err := wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, true, func(context.Context) (bool, error) {
		value, _, err := pc.Query(context.Background(), metricName, time.Time{})
		if err != nil {
			return false, err
		}
		vec, ok := value.(prommodel.Vector)
		if !ok || len(vec) == 0 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("metric %q not found in Prometheus: %w", metricName, err)
	}
	return nil
}

// VerifyMetricAbsent verifies that a metric does NOT exist in Prometheus.
// Uses retry logic to handle cases where stale metric data might persist briefly
// until the next Prometheus scrape interval confirms the metric is no longer emitted.
func VerifyMetricAbsent(cs *clients.Clients, metricName string) error {
	pc, err := newPrometheusClient(cs)
	if err != nil {
		return fmt.Errorf("failed to create Prometheus client while checking absence of %q: %w", metricName, err)
	}
	if err := wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, true, func(context.Context) (bool, error) {
		value, _, err := pc.Query(context.Background(), metricName, time.Time{})
		if err != nil {
			return false, err
		}
		vec, ok := value.(prommodel.Vector)
		if !ok || len(vec) == 0 {
			return true, nil // Metric is absent - success
		}
		return false, nil // Metric still present - keep retrying
	}); err != nil {
		return fmt.Errorf("metric %q should not exist but is still present after timeout: %w", metricName, err)
	}
	return nil
}

func getBearerTokenForPrometheusAccount(cs *clients.Clients) (string, error) {
	secrets, err := cs.KubeClient.Kube.CoreV1().Secrets("openshift-monitoring").List(context.Background(), meta.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting secrets from namespace %v: %w", "openshift-monitoring", err)
	}
	tokenSecret := getPrometheusSecretNameForToken(secrets.Items)
	if tokenSecret == "" {
		// generate token for service account prometheus-k8s
		output := cmd.Run("oc", "sa", "new-token", "prometheus-k8s", "-n", "openshift-monitoring")
		if output.ExitCode != 0 {
			return "", fmt.Errorf("error creating token for the service account prometheus-k8s: %v", output.Stderr())
		}
		secrets, err := cs.KubeClient.Kube.CoreV1().Secrets("openshift-monitoring").List(context.Background(), meta.ListOptions{})
		if err != nil {
			return "", fmt.Errorf("error getting secrets from namespace %v: %w", "openshift-monitoring", err)
		}
		tokenSecret = getPrometheusSecretNameForToken(secrets.Items)
		if tokenSecret == "" {
			return "", errors.New("could not find a service account token for service account \"prometheus-k8s\"")
		}
	}
	sec, err := cs.KubeClient.Kube.CoreV1().Secrets("openshift-monitoring").Get(context.Background(), tokenSecret, meta.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting secret %s %w", tokenSecret, err)
	}
	tokenContents := sec.Data["token"]
	if len(tokenContents) == 0 {
		return "", fmt.Errorf("token data is missing for token %s", tokenSecret)
	}
	return string(tokenContents), nil
}

func getPrometheusSecretNameForToken(secrets []corev1.Secret) string {
	for _, sec := range secrets {
		if strings.Contains(sec.Name, "prometheus-k8s") {
			if strings.Contains(sec.Name, "token") {
				return sec.Name
			}
		}
	}
	return ""
}
