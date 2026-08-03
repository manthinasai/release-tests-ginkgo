package pipelines

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/wait"
)

// ValidateTaskRun validates a TaskRun by matching name pattern and checking status.
func ValidateTaskRun(c *clients.Clients, trname, status, namespace string) {
	matchedTrname := getTaskRunNameMatches(c, trname, namespace)
	if matchedTrname == "" {
		Fail(fmt.Sprintf("Error: Nothing matched with Taskrun name: %s in namespace %s", trname, namespace))
	}
	// Verify status of TaskRun (wait for it)
	switch {
	case strings.Contains(strings.ToLower(status), "success"):
		validateTaskRunForSuccessStatus(c, matchedTrname, namespace)
	case strings.Contains(strings.ToLower(status), "fail"):
		validateTaskRunForFailedStatus(c, matchedTrname, namespace)
	case strings.Contains(strings.ToLower(status), "timeout"):
		validateTaskRunTimeOutFailure(c, matchedTrname, namespace)
	default:
		Fail(fmt.Sprintf("Error: Not valid status input: %s", status))
	}
}

func validateTaskRunForFailedStatus(c *clients.Clients, trname, namespace string) {
	log.Printf("Waiting for TaskRun %s in namespace %s to fail", trname, namespace)
	err := wait.WaitForTaskRunState(c, trname, wait.TaskRunFailed(trname), "BuildValidationFailed")
	if err != nil {
		logs := getTaskrunLogs(trname, namespace)
		events, eventError := k8s.GetWarningEvents(c, namespace)
		if eventError != nil {
			Fail(fmt.Sprintf("task run %s was expected to be in TaskRunFailed state \n%v \ntaskrun logs: \n%s \ntaskrun events error: \n%v", trname, err, logs, eventError))
		} else {
			Fail(fmt.Sprintf("task run %s was expected to be in TaskRunFailed state \n%v \ntaskrun logs: \n%s \ntaskrun events: \n%v", trname, err, logs, events))
		}
	}
}

func validateTaskRunForSuccessStatus(c *clients.Clients, trname, namespace string) {
	log.Printf("Waiting for TaskRun %s in namespace %s to succeed", trname, namespace)
	err := wait.WaitForTaskRunState(c, trname, wait.TaskRunSucceed(trname), "TaskRunSucceed")
	if err != nil {
		logs := getTaskrunLogs(trname, namespace)
		events, eventError := k8s.GetWarningEvents(c, namespace)
		if eventError != nil {
			Fail(fmt.Sprintf("task run %s was expected to be in TaskRunSucceed state \n%v \ntaskrun logs: \n%s \ntaskrun events error: \n%v", trname, err, logs, eventError))
		} else {
			Fail(fmt.Sprintf("task run %s was expected to be in TaskRunSucceed state \n%v \ntaskrun logs: \n%s \ntaskrun events: \n%v", trname, err, logs, events))
		}
	}
}

func validateTaskRunTimeOutFailure(c *clients.Clients, trname, namespace string) {
	log.Printf("Waiting for TaskRun %s in namespace %s to complete", trname, namespace)
	err := wait.WaitForTaskRunState(c, trname, wait.FailedWithReason("TaskRunTimeout", trname), "TaskRunTimeout")
	if err != nil {
		Fail(fmt.Sprintf("task run %s was expected to be in TaskRunTimeout state \n %v", trname, err))
	}
}

func getTaskRunNameMatches(c *clients.Clients, trname, namespace string) string {
	trlist, err := c.TaskRunClient.List(c.Ctx, metav1.ListOptions{})
	if err != nil {
		Fail(fmt.Sprintf("failed to list task runs in namespace %s \n %v", namespace, err))
	}

	var matchedTr string
	match, _ := regexp.Compile(trname + ".*?")
	for _, tr := range trlist.Items {
		if match.MatchString(tr.Name) {
			matchedTr = tr.Name
			break
		}
	}
	return matchedTr
}

// getTaskrunLogs retrieves task run logs using the oc CLI.
func getTaskrunLogs(trname, namespace string) string {
	result := cmd.Run("oc", "logs", "--selector=tekton.dev/taskRun="+trname, "-n", namespace, "--all-containers", "--ignore-errors=true")
	return result.Stdout()
}
