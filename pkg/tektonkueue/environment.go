// Package tektonkueue configures and validates multi-cluster PipelineRun execution.
package tektonkueue

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	operatorv1alpha1 "github.com/tektoncd/operator/pkg/apis/operator/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

const (
	kueueNamespace = "openshift-kueue-operator"
	pollInterval   = 3 * time.Second
)

var kueueOperatorGVR = schema.GroupVersionResource{Group: "kueue.openshift.io", Version: "v1", Resource: "kueues"}

// Cluster identifies one configured test cluster without exposing its API endpoint.
type Cluster struct {
	Name    string
	Clients *clients.Clients
}

type cleanupFunc func(context.Context) error

// Environment owns the temporary resources for one multi-cluster workload test.
type Environment struct {
	Hub       Cluster
	Spokes    []Cluster
	Prefix    string
	Namespace string

	runName      string
	workloadName string
	cleanups     []cleanupFunc
}

// NewEnvironment creates a test environment. Setup performs validation before mutating clusters.
func NewEnvironment(hub Cluster, spokes []Cluster, prefix string) *Environment {
	return &Environment{
		Hub:       hub,
		Spokes:    spokes,
		Prefix:    prefix,
		Namespace: prefix,
	}
}

// Setup configures Kueue, scoped worker credentials, queues, and hub/spoke scheduler roles.
func (e *Environment) Setup(ctx context.Context) error {
	if err := e.validate(); err != nil {
		return err
	}

	for _, cluster := range e.clusters() {
		if err := e.ensureKueue(ctx, cluster); err != nil {
			return fmt.Errorf("configure Kueue on %s: %w", cluster.Name, err)
		}
	}
	for _, cluster := range e.clusters() {
		if err := e.waitForKueue(ctx, cluster); err != nil {
			return fmt.Errorf("wait for Kueue on %s: %w", cluster.Name, err)
		}
	}

	for _, cluster := range e.clusters() {
		if err := e.createNamespace(ctx, cluster); err != nil {
			return fmt.Errorf("create namespace on %s: %w", cluster.Name, err)
		}
		cluster.Clients.NewClientSet(e.Namespace)
		if err := e.createKueueControllerRBAC(ctx, cluster); err != nil {
			return fmt.Errorf("configure Kueue RBAC on %s: %w", cluster.Name, err)
		}
	}
	for _, spoke := range e.Spokes {
		if err := e.createQueues(ctx, spoke, false); err != nil {
			return fmt.Errorf("create queues on %s: %w", spoke.Name, err)
		}
		if err := e.createWorkerCredential(ctx, spoke); err != nil {
			return fmt.Errorf("create scoped credentials for %s: %w", spoke.Name, err)
		}
	}
	if err := e.createHubMultiKueueResources(ctx); err != nil {
		return err
	}
	if err := e.createQueues(ctx, e.Hub, true); err != nil {
		return fmt.Errorf("create queues on hub: %w", err)
	}
	if err := e.createKueueEgressPolicy(ctx); err != nil {
		return err
	}

	for _, cluster := range e.clusters() {
		role := operatorv1alpha1.MultiClusterRoleSpoke
		if cluster.Name == e.Hub.Name {
			role = operatorv1alpha1.MultiClusterRoleHub
		}
		if err := e.enableScheduler(ctx, cluster, role); err != nil {
			return fmt.Errorf("enable scheduler on %s: %w", cluster.Name, err)
		}
	}

	return e.waitForKueueResources(ctx)
}

// Cleanup restores global configuration and removes every test-owned resource.
func (e *Environment) Cleanup(ctx context.Context) error {
	cleanups := e.cleanups
	e.cleanups = nil

	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := cleanups[i](ctx); err != nil {
			e.cleanups = append([]cleanupFunc(nil), cleanups[:i+1]...)
			return err
		}
	}
	return nil
}

func (e *Environment) addCleanup(cleanup cleanupFunc) {
	e.cleanups = append(e.cleanups, cleanup)
}

func (e *Environment) clusters() []Cluster {
	return append([]Cluster{e.Hub}, e.Spokes...)
}

func (e *Environment) validate() error {
	if errs := validation.IsDNS1123Label(e.Prefix); len(errs) != 0 {
		return fmt.Errorf("invalid test prefix %q: %s", e.Prefix, strings.Join(errs, ", "))
	}
	if len(e.Spokes) == 0 {
		return fmt.Errorf("at least one spoke is required")
	}

	hosts := make(map[string]string, len(e.Spokes)+1)
	for _, cluster := range e.clusters() {
		if cluster.Name == "" || cluster.Clients == nil || cluster.Clients.KubeConfig == nil {
			return fmt.Errorf("cluster name, clients, and REST config are required")
		}
		host := strings.TrimSuffix(cluster.Clients.KubeConfig.Host, "/")
		if previous, exists := hosts[host]; exists {
			return fmt.Errorf("clusters %s and %s use the same API server", previous, cluster.Name)
		}
		hosts[host] = cluster.Name
	}
	return nil
}

func (e *Environment) ensureKueue(ctx context.Context, cluster Cluster) error {
	resource := cluster.Clients.Dynamic.Resource(kueueOperatorGVR)
	existing, err := resource.Get(ctx, "cluster", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		var uid types.UID
		e.addCleanup(func(cleanupCtx context.Context) error {
			current, getErr := resource.Get(cleanupCtx, "cluster", metav1.GetOptions{})
			if apierrors.IsNotFound(getErr) {
				return nil
			}
			if getErr != nil {
				return fmt.Errorf("get test-created Kueue on %s: %w", cluster.Name, getErr)
			}
			if !e.owns(current.GetLabels()) || (uid != "" && current.GetUID() != uid) {
				return fmt.Errorf("refusing to delete replacement Kueue on %s", cluster.Name)
			}
			if err := ignoreNotFound(resource.Delete(cleanupCtx, "cluster", metav1.DeleteOptions{})); err != nil {
				return err
			}
			return waitForNotFound(cleanupCtx, 10*time.Minute, func(ctx context.Context) error {
				_, err := resource.Get(ctx, "cluster", metav1.GetOptions{})
				return err
			})
		})
		created, createErr := resource.Create(ctx, e.requiredKueue(), metav1.CreateOptions{})
		if createErr != nil {
			return createErr
		}
		uid = created.GetUID()
		return nil
	}
	if err != nil {
		return err
	}

	changedPaths, err := ensurePipelineRunIntegration(existing)
	if err != nil {
		return err
	}
	if len(changedPaths) == 0 {
		return nil
	}
	e.addCleanup(func(cleanupCtx context.Context) error {
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current, getErr := resource.Get(cleanupCtx, "cluster", metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			changed := false
			for _, path := range changedPaths {
				items, _, nestedErr := unstructured.NestedSlice(current.Object, path...)
				if nestedErr != nil {
					return nestedErr
				}
				items, removed := removePipelineRunFramework(items)
				if removed {
					if setErr := unstructured.SetNestedSlice(current.Object, items, path...); setErr != nil {
						return setErr
					}
					changed = true
				}
			}
			if !changed {
				return nil
			}
			_, updateErr := resource.Update(cleanupCtx, current, metav1.UpdateOptions{})
			return updateErr
		})
	})
	_, err = resource.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (e *Environment) requiredKueue() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kueue.openshift.io/v1",
		"kind":       "Kueue",
		"metadata": map[string]any{
			"name":   "cluster",
			"labels": e.ownedLabels(),
		},
		"spec": map[string]any{
			"config": map[string]any{
				"integrations": map[string]any{
					"externalFrameworks": []any{pipelineRunFramework()},
					// The CRD requires one built-in framework. Deployment has no MultiKueue
					// adapter, so the scoped worker credential only needs PipelineRun access.
					"frameworks": []any{"Deployment"},
				},
				"multiKueue": map[string]any{
					"externalFrameworks": []any{pipelineRunFramework()},
				},
			},
			"logLevel":         "Normal",
			"managementState":  "Managed",
			"operatorLogLevel": "Normal",
		},
	}}
}

func pipelineRunFramework() map[string]any {
	return map[string]any{"group": "tekton.dev", "resource": "pipelineruns", "version": "v1"}
}

func ensurePipelineRunIntegration(kueue *unstructured.Unstructured) ([][]string, error) {
	var changedPaths [][]string
	for _, path := range [][]string{{"spec", "config", "integrations", "externalFrameworks"}, {"spec", "config", "multiKueue", "externalFrameworks"}} {
		items, found, err := unstructured.NestedSlice(kueue.Object, path...)
		if err != nil {
			return nil, err
		}
		if !found || !slices.ContainsFunc(items, isPipelineRunFramework) {
			items = append(items, pipelineRunFramework())
			if err := unstructured.SetNestedSlice(kueue.Object, items, path...); err != nil {
				return nil, err
			}
			changedPaths = append(changedPaths, path)
		}
	}
	return changedPaths, nil
}

func removePipelineRunFramework(items []any) ([]any, bool) {
	for i, item := range items {
		if isPipelineRunFramework(item) {
			return slices.Delete(items, i, i+1), true
		}
	}
	return items, false
}

func isPipelineRunFramework(item any) bool {
	framework, ok := item.(map[string]any)
	return ok && framework["group"] == "tekton.dev" && framework["resource"] == "pipelineruns" && framework["version"] == "v1"
}

func (e *Environment) waitForKueue(ctx context.Context, cluster Cluster) error {
	resource := cluster.Clients.Dynamic.Resource(kueueOperatorGVR)
	var last string
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		object, err := resource.Get(ctx, "cluster", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		ready, availableDetail := conditionTrue(object, "Available")
		_, degradedDetail := conditionTrue(object, "Degraded")
		last = fmt.Sprintf("available=%s, degraded=%s", availableDetail, degradedDetail)
		return ready, nil
	})
	if err != nil {
		return fmt.Errorf("kueue did not become available (%s): %w", last, err)
	}
	return nil
}

func (e *Environment) enableScheduler(ctx context.Context, cluster Cluster, role operatorv1alpha1.MultiClusterRole) error {
	client := cluster.Clients.TektonConfig()
	current, err := client.Get(ctx, "config", metav1.GetOptions{})
	if err != nil {
		return err
	}
	original := current.Spec.Scheduler.DeepCopy()
	e.addCleanup(func(cleanupCtx context.Context) error {
		var conflicts []error
		updateErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latest, getErr := client.Get(cleanupCtx, "config", metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			conflicts = restoreSchedulerFields(&latest.Spec.Scheduler, original, role, e.Prefix)
			_, err := client.Update(cleanupCtx, latest, metav1.UpdateOptions{})
			return err
		})
		readyErr := waitForTektonConfig(cleanupCtx, cluster)
		if updateErr != nil {
			updateErr = fmt.Errorf("restore scheduler on %s: %w", cluster.Name, updateErr)
		}
		return errors.Join(updateErr, readyErr, errors.Join(conflicts...))
	})

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest, getErr := client.Get(ctx, "config", metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		disabled := false
		latest.Spec.Scheduler.Disabled = &disabled
		latest.Spec.Scheduler.MultiClusterDisabled = false
		latest.Spec.Scheduler.MultiClusterRole = role
		latest.Spec.Scheduler.QueueName = e.Prefix
		latest.Spec.Scheduler.MultiKueueOverride = role == operatorv1alpha1.MultiClusterRoleHub
		_, updateErr := client.Update(ctx, latest, metav1.UpdateOptions{})
		return updateErr
	}); err != nil {
		return err
	}
	return waitForTektonConfig(ctx, cluster)
}

func restoreSchedulerFields(current, original *operatorv1alpha1.Scheduler, role operatorv1alpha1.MultiClusterRole, queueName string) []error {
	var conflicts []error
	if !equalBoolPointers(current.Disabled, original.Disabled) {
		if current.Disabled != nil && !*current.Disabled {
			if original.Disabled == nil {
				current.Disabled = nil
			} else {
				value := *original.Disabled
				current.Disabled = &value
			}
		} else {
			conflicts = append(conflicts, fmt.Errorf("scheduler.disabled changed concurrently"))
		}
	}
	if current.MultiClusterDisabled != original.MultiClusterDisabled {
		if !current.MultiClusterDisabled {
			current.MultiClusterDisabled = original.MultiClusterDisabled
		} else {
			conflicts = append(conflicts, fmt.Errorf("scheduler.multi-cluster-disabled changed concurrently"))
		}
	}
	if current.MultiClusterRole != original.MultiClusterRole {
		if current.MultiClusterRole == role {
			current.MultiClusterRole = original.MultiClusterRole
		} else {
			conflicts = append(conflicts, fmt.Errorf("scheduler.multi-cluster-role changed concurrently"))
		}
	}
	if current.QueueName != original.QueueName {
		if current.QueueName == queueName {
			current.QueueName = original.QueueName
		} else {
			conflicts = append(conflicts, fmt.Errorf("scheduler queueName changed concurrently"))
		}
	}
	expectedOverride := role == operatorv1alpha1.MultiClusterRoleHub
	if current.MultiKueueOverride != original.MultiKueueOverride {
		if current.MultiKueueOverride == expectedOverride {
			current.MultiKueueOverride = original.MultiKueueOverride
		} else {
			conflicts = append(conflicts, fmt.Errorf("scheduler multiKueueOverride changed concurrently"))
		}
	}
	return conflicts
}

func equalBoolPointers(left, right *bool) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func waitForTektonConfig(ctx context.Context, cluster Cluster) error {
	configs := cluster.Clients.TektonConfig()
	schedulers := cluster.Clients.Operator.TektonSchedulers()
	var last string
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		config, err := configs.Get(ctx, "config", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		for _, condition := range config.Status.Conditions {
			if condition.Type == "Ready" {
				last = fmt.Sprintf("TektonConfig %s: %s", condition.Reason, condition.Message)
			}
		}
		if !config.Spec.Scheduler.IsDisabled() {
			if terminalErr := schedulerTerminalError(ctx, cluster); terminalErr != nil {
				return false, terminalErr
			}
		}
		if !config.Status.IsReady() {
			return false, nil
		}
		if config.Spec.Scheduler.IsDisabled() {
			return true, nil
		}

		scheduler, err := schedulers.Get(ctx, operatorv1alpha1.TektonSchedulerResourceName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		for _, condition := range scheduler.Status.Conditions {
			if condition.Type == "Ready" {
				last = fmt.Sprintf("TektonScheduler %s: %s", condition.Reason, condition.Message)
			}
		}
		return scheduler.Status.IsReady(), nil
	})
	if err != nil {
		return fmt.Errorf("scheduler on %s did not become ready (%s): %w", cluster.Name, last, err)
	}
	return nil
}

func schedulerTerminalError(ctx context.Context, cluster Cluster) error {
	pods, err := cluster.Clients.KubeClient.Kube.CoreV1().Pods(config.TargetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	for i := range pods.Items {
		if !strings.HasPrefix(pods.Items[i].Name, "tekton-kueue-") {
			continue
		}
		for _, status := range pods.Items[i].Status.ContainerStatuses {
			waiting := status.State.Waiting
			if waiting != nil && waiting.Reason == "CreateContainerError" && strings.Contains(waiting.Message, "executable file") {
				return fmt.Errorf("tekton scheduler pod on %s cannot start: %s", cluster.Name, waiting.Message)
			}
		}
	}
	return nil
}

func conditionTrue(object *unstructured.Unstructured, conditionType string) (bool, string) {
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok || condition["type"] != conditionType {
			continue
		}
		detail := fmt.Sprintf("%v: %v", condition["reason"], condition["message"])
		return condition["status"] == "True", detail
	}
	return false, "condition not reported"
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
