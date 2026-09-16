package tektonkueue

import (
	"context"
	"fmt"
	"time"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"knative.dev/pkg/apis"
)

const (
	multiKueueController = "kueue.x-k8s.io/multikueue"
	queueLabel           = "kueue.x-k8s.io/queue-name"
	workloadImage        = "registry.access.redhat.com/ubi9/ubi-minimal@sha256:34880b64c07f28f64d95737f82f891516de9a3b43583f39970f7bf8e4cfa48b7"
)

// Result identifies the selected spoke without exposing its API endpoint.
type Result struct {
	PipelineRun string
	Spoke       string
}

// Execute creates one hub PipelineRun and proves it runs on exactly one spoke.
func (e *Environment) Execute(ctx context.Context) (*Result, error) {
	run, err := e.createPipelineRun(ctx)
	if err != nil {
		return nil, err
	}
	workloadName, err := e.waitForHubWorkload(ctx, run)
	if err != nil {
		return nil, err
	}
	spoke, err := e.waitForSpokeExecution(ctx, run.Name)
	if err != nil {
		return nil, err
	}
	if err := e.waitForCompletion(ctx, run.Name, workloadName, spoke); err != nil {
		return nil, err
	}
	return &Result{PipelineRun: run.Name, Spoke: spoke.Name}, nil
}

func (e *Environment) createPipelineRun(ctx context.Context) (*pipelinev1.PipelineRun, error) {
	managedBy := multiKueueController
	timeout := metav1.Duration{Duration: 10 * time.Minute}
	labels := e.ownedLabels()
	labels[queueLabel] = e.Prefix
	run := &pipelinev1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      e.Prefix + "-run",
			Namespace: e.Namespace,
			Labels:    labels,
		},
		Spec: pipelinev1.PipelineRunSpec{
			ManagedBy:       &managedBy,
			Timeouts:        &pipelinev1.TimeoutFields{Pipeline: &timeout},
			TaskRunTemplate: pipelinev1.PipelineTaskRunTemplate{ServiceAccountName: "default"},
			PipelineSpec: &pipelinev1.PipelineSpec{Tasks: []pipelinev1.PipelineTask{{
				Name: "execute-on-spoke",
				TaskSpec: &pipelinev1.EmbeddedTask{TaskSpec: pipelinev1.TaskSpec{Steps: []pipelinev1.Step{{
					Name:   "prove-execution",
					Image:  workloadImage,
					Script: "#!/bin/sh\necho multi-cluster-execution-ok\nsleep 45\n",
				}}}},
			}}},
		},
	}
	e.runName = run.Name
	var uid types.UID
	e.addCleanup(func(cleanupCtx context.Context) error {
		current, getErr := e.Hub.Clients.PipelineRunClient.Get(cleanupCtx, run.Name, metav1.GetOptions{})
		if getErr == nil {
			if !e.owns(current.Labels) || (uid != "" && current.UID != uid) {
				return fmt.Errorf("refusing to delete replacement PipelineRun %s", run.Name)
			}
			if err := ignoreNotFound(e.Hub.Clients.PipelineRunClient.Delete(cleanupCtx, run.Name, metav1.DeleteOptions{})); err != nil {
				return err
			}
		} else if !apierrors.IsNotFound(getErr) {
			return getErr
		}
		return e.waitForExecutionObjectsGone(cleanupCtx)
	})
	created, err := e.Hub.Clients.PipelineRunClient.Create(ctx, run, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create hub PipelineRun: %w", err)
	}
	uid = created.UID
	return created, nil
}

func (e *Environment) waitForHubWorkload(ctx context.Context, run *pipelinev1.PipelineRun) (string, error) {
	resource := e.Hub.Clients.Dynamic.Resource(workloadGVR).Namespace(e.Namespace)
	var name string
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		list, err := resource.List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, nil
		}
		for i := range list.Items {
			for _, owner := range list.Items[i].GetOwnerReferences() {
				if owner.UID == run.UID {
					name = list.Items[i].GetName()
					return true, nil
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("hub Workload was not created for PipelineRun %s: %w", run.Name, err)
	}
	e.workloadName = name
	return name, nil
}

func (e *Environment) waitForSpokeExecution(ctx context.Context, runName string) (Cluster, error) {
	seen := map[string]bool{}
	var selected Cluster
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		for _, spoke := range e.Spokes {
			if _, err := spoke.Clients.PipelineRunClient.Get(ctx, runName, metav1.GetOptions{}); err == nil {
				seen[spoke.Name] = true
				selected = spoke
			} else if !apierrors.IsNotFound(err) {
				return false, err
			}
		}
		if len(seen) > 1 {
			return false, fmt.Errorf("PipelineRun appeared on multiple spokes")
		}
		if len(seen) == 0 {
			return false, nil
		}
		taskRuns, err := selected.Clients.TaskRunClient.List(ctx, metav1.ListOptions{LabelSelector: "tekton.dev/pipelineRun=" + runName})
		if err != nil {
			return false, err
		}
		return len(taskRuns.Items) > 0, nil
	})
	if err != nil {
		return Cluster{}, fmt.Errorf("PipelineRun %s did not execute on exactly one spoke: %w", runName, err)
	}

	hubTaskRuns, err := e.Hub.Clients.TaskRunClient.List(ctx, metav1.ListOptions{LabelSelector: "tekton.dev/pipelineRun=" + runName})
	if err != nil {
		return Cluster{}, err
	}
	if len(hubTaskRuns.Items) != 0 {
		return Cluster{}, fmt.Errorf("PipelineRun %s unexpectedly created TaskRuns on the hub", runName)
	}
	return selected, nil
}

func (e *Environment) waitForCompletion(ctx context.Context, runName, workloadName string, spoke Cluster) error {
	workerObservedSuccess := false
	seen := map[string]bool{spoke.Name: true}
	var hubState, workerState string
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err := e.recordSpokes(ctx, runName, seen); err != nil {
			return false, err
		}

		worker, workerErr := spoke.Clients.PipelineRunClient.Get(ctx, runName, metav1.GetOptions{})
		if workerErr == nil {
			terminal, succeeded, state := pipelineRunState(worker)
			workerState = state
			if terminal && !succeeded {
				return false, fmt.Errorf("PipelineRun failed on %s (%s)", spoke.Name, state)
			}
			workerObservedSuccess = workerObservedSuccess || succeeded
		} else if !apierrors.IsNotFound(workerErr) {
			return false, workerErr
		}

		hub, hubErr := e.Hub.Clients.PipelineRunClient.Get(ctx, runName, metav1.GetOptions{})
		if hubErr != nil {
			return false, hubErr
		}
		terminal, succeeded, state := pipelineRunState(hub)
		hubState = state
		if terminal && !succeeded {
			return false, fmt.Errorf("hub PipelineRun failed after execution on %s (%s)", spoke.Name, state)
		}
		return succeeded && workerObservedSuccess, nil
	})
	if err != nil {
		return fmt.Errorf("PipelineRun %s did not complete (hub=%s, %s=%s): %w", runName, hubState, spoke.Name, workerState, err)
	}

	if err := e.waitForWorkloadFinished(ctx, workloadName, runName, seen); err != nil {
		return err
	}
	if err := e.waitForWorkerCleanup(ctx, spoke, runName, workloadName, seen); err != nil {
		return err
	}

	hubTaskRuns, err := e.Hub.Clients.TaskRunClient.List(ctx, metav1.ListOptions{LabelSelector: "tekton.dev/pipelineRun=" + runName})
	if err != nil {
		return err
	}
	if len(hubTaskRuns.Items) != 0 {
		return fmt.Errorf("PipelineRun %s created TaskRuns on the hub", runName)
	}
	return nil
}

func (e *Environment) recordSpokes(ctx context.Context, runName string, seen map[string]bool) error {
	for _, candidate := range e.Spokes {
		if _, err := candidate.Clients.PipelineRunClient.Get(ctx, runName, metav1.GetOptions{}); err == nil {
			seen[candidate.Name] = true
		} else if !apierrors.IsNotFound(err) {
			return err
		}
	}
	if len(seen) > 1 {
		return fmt.Errorf("PipelineRun appeared on multiple spokes")
	}
	return nil
}

func pipelineRunState(run *pipelinev1.PipelineRun) (terminal, succeeded bool, detail string) {
	condition := run.Status.GetCondition(apis.ConditionSucceeded)
	if condition == nil {
		return false, false, "condition not reported"
	}
	detail = fmt.Sprintf("%s: %s", condition.Reason, condition.Message)
	switch condition.Status {
	case corev1.ConditionTrue:
		return true, true, detail
	case corev1.ConditionFalse:
		return true, false, detail
	default:
		return false, false, detail
	}
}

func (e *Environment) waitForWorkloadFinished(ctx context.Context, workloadName, runName string, seen map[string]bool) error {
	resource := e.Hub.Clients.Dynamic.Resource(workloadGVR).Namespace(e.Namespace)
	var last string
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err := e.recordSpokes(ctx, runName, seen); err != nil {
			return false, err
		}
		workload, getErr := resource.Get(ctx, workloadName, metav1.GetOptions{})
		if getErr != nil {
			return false, nil
		}
		finished, detail := conditionTrue(workload, "Finished")
		last = detail
		return finished, nil
	})
	if err != nil {
		return fmt.Errorf("hub Workload %s did not finish (%s): %w", workloadName, last, err)
	}
	return nil
}

func (e *Environment) waitForExecutionObjectsGone(ctx context.Context) error {
	hubWorkloads := e.Hub.Clients.Dynamic.Resource(workloadGVR).Namespace(e.Namespace)
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		if _, err := e.Hub.Clients.PipelineRunClient.Get(ctx, e.runName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
			return false, ignoreNotFound(err)
		}
		if e.workloadName != "" {
			if _, err := hubWorkloads.Get(ctx, e.workloadName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
				return false, ignoreNotFound(err)
			}
		}
		for _, spoke := range e.Spokes {
			if _, err := spoke.Clients.PipelineRunClient.Get(ctx, e.runName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
				return false, ignoreNotFound(err)
			}
			if e.workloadName != "" {
				workloads := spoke.Clients.Dynamic.Resource(workloadGVR).Namespace(e.Namespace)
				if _, err := workloads.Get(ctx, e.workloadName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
					return false, ignoreNotFound(err)
				}
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("execution objects were not deleted before scheduler restoration: %w", err)
	}
	return nil
}

func (e *Environment) waitForWorkerCleanup(ctx context.Context, spoke Cluster, runName, workloadName string, seen map[string]bool) error {
	workloads := spoke.Clients.Dynamic.Resource(workloadGVR).Namespace(e.Namespace)
	err := wait.PollUntilContextTimeout(ctx, pollInterval, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err := e.recordSpokes(ctx, runName, seen); err != nil {
			return false, err
		}
		_, runErr := spoke.Clients.PipelineRunClient.Get(ctx, runName, metav1.GetOptions{})
		_, workloadErr := workloads.Get(ctx, workloadName, metav1.GetOptions{})
		return apierrors.IsNotFound(runErr) && apierrors.IsNotFound(workloadErr), nil
	})
	if err != nil {
		return fmt.Errorf("worker objects were not cleaned from %s: %w", spoke.Name, err)
	}
	return nil
}
