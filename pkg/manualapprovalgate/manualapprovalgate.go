/*
Copyright 2020 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package approvalgate provides helpers for managing Manual Approval Gate resources.
package approvalgate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/gomega" //nolint:revive,staticcheck // dot import is idiomatic for Gomega
	atv1alpha1 "github.com/openshift-pipelines/manual-approval-gate/pkg/apis/approvaltask/v1alpha1"
	operatorv1alpha1 "github.com/tektoncd/operator/pkg/apis/operator/v1alpha1"
	mag "github.com/tektoncd/operator/pkg/client/clientset/versioned/typed/operator/v1alpha1"
	"github.com/tektoncd/operator/test/utils"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/cmd"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
)

// ApprovalTaskInfo holds summary information about a Manual Approval Gate task.
type ApprovalTaskInfo struct {
	Name   string
	Status string
}

// EnsureManualApprovalGateExists waits until the ManualApprovalGate CR exists and returns it.
func EnsureManualApprovalGateExists(clients mag.ManualApprovalGateInterface, names utils.ResourceNames) (*operatorv1alpha1.ManualApprovalGate, error) {
	var magCR *operatorv1alpha1.ManualApprovalGate

	err := wait.PollUntilContextTimeout(context.TODO(), config.APIRetry, config.APITimeout, false, func(ctx context.Context) (bool, error) {
		cr, err := clients.Get(ctx, names.ManualApprovalGate, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				log.Printf("Waiting for availability of manual approval gate cr [%s]\n", names.ManualApprovalGate)
				return false, nil
			}
			return false, err
		}
		magCR = cr
		return true, nil
	})

	return magCR, err
}

// ValidateMAGDeployment ensures the ManualApprovalGate CR is ready.
func ValidateMAGDeployment(cs *clients.Clients) {
	names := utils.ResourceNames{ManualApprovalGate: "manual-approval-gate"}
	_, err := EnsureManualApprovalGateExists(cs.ManualApprovalGate(), names)
	Expect(err).NotTo(HaveOccurred(), "ManualApprovalGate CR not ready")
}

// ListApprovalTask polls until approval tasks are available and returns their info.
func ListApprovalTask(cs *clients.Clients) ([]ApprovalTaskInfo, error) {
	var tasks []ApprovalTaskInfo

	err := wait.PollUntilContextTimeout(cs.Ctx, config.APIRetry, config.APITimeout, false, func(ctx context.Context) (bool, error) {
		at, err := cs.ApprovalTask.List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Failed to list approval tasks, retrying...: %v", err)
			return false, err
		}

		if len(at.Items) == 0 {
			log.Printf("No approval tasks found, retrying...")
			return false, nil
		}

		tasks = make([]ApprovalTaskInfo, 0, len(at.Items))
		for _, item := range at.Items {
			tasks = append(tasks, ApprovalTaskInfo{
				Name:   item.Name,
				Status: item.Status.State,
			})
		}

		return true, nil
	})
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

// ValidateApprovalGatePipeline checks if any approval task matches the expected status.
func ValidateApprovalGatePipeline(cs *clients.Clients, expectedStatus string) (bool, error) {
	tasks, err := ListApprovalTask(cs)
	if err != nil {
		return false, fmt.Errorf("error fetching approval tasks: %w", err)
	}

	for _, task := range tasks {
		actualStatus := checkApprovalTaskStatus(task)
		if actualStatus == expectedStatus {
			return true, nil
		}
	}

	return false, errors.New("no approval tasks were found in the specified state")
}

func checkApprovalTaskStatus(task ApprovalTaskInfo) string {
	switch task.Status {
	case "pending":
		return "Pending"
	case "rejected":
		return "Rejected"
	case "approved":
		return "Approved"
	default:
		return "Unknown Error: Check Details"
	}
}

// ApproveApprovalGatePipeline approves the named approval task via the opc CLI.
func ApproveApprovalGatePipeline(taskname string) {
	cmd.MustSucceed("opc", "approvaltask", "approve", taskname)
}

// RejectApprovalGatePipeline rejects the named approval task via the opc CLI.
func RejectApprovalGatePipeline(taskname string) {
	cmd.MustSucceed("opc", "approvaltask", "reject", taskname)
}

// ── Group User Helpers ────────────────────────────────────────────────────────

var (
	magAPIServerOnce sync.Once
	magAPIServer     string
	magAPIServerErr  error

	magUserKubeconfigsMu sync.Mutex
	magUserKubeconfigs   = map[string]string{}

	magUserAuthDirtyMu sync.Mutex
	magUserAuthDirty   = map[string]bool{}
)

func markUsersAuthDirty(users []string) {
	magUserAuthDirtyMu.Lock()
	defer magUserAuthDirtyMu.Unlock()
	for _, u := range users {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		magUserAuthDirty[u] = true
	}
}

func popUserAuthDirty(user string) bool {
	magUserAuthDirtyMu.Lock()
	defer magUserAuthDirtyMu.Unlock()
	dirty := magUserAuthDirty[user]
	if dirty {
		magUserAuthDirty[user] = false
	}
	return dirty
}

func userPassword(user string) string {
	envVar := strings.ToUpper(user) + "_PASS"
	if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
		return v
	}
	return user
}

// MAGGroupName returns a namespace-scoped unique group name for the given alias.
// OpenShift Groups are cluster-scoped; prefixing with the namespace avoids collisions
// between parallel tests.
func MAGGroupName(namespace, alias string) string {
	a := strings.TrimSpace(alias)
	if a == "" {
		return ""
	}
	if strings.Contains(a, ":") {
		return a
	}

	a = strings.ToLower(a)
	a = strings.ReplaceAll(a, "_", "-")
	a = strings.ReplaceAll(a, " ", "-")

	var b strings.Builder
	b.Grow(len(a))
	lastDash := false
	for _, r := range a {
		isAZ := r >= 'a' && r <= 'z'
		is09 := r >= '0' && r <= '9'
		if isAZ || is09 {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' {
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	safeAlias := strings.Trim(b.String(), "-")
	if safeAlias == "" {
		safeAlias = "group"
	}
	return fmt.Sprintf("mag-%s-%s", namespace, safeAlias)
}

// EnsureGroupMembers ensures the named OpenShift Group exists with exactly the provided members.
func EnsureGroupMembers(group string, users []string) {
	Expect(group).NotTo(BeEmpty(), "group name must not be empty")

	oldUsers := []string{}
	getUsersCmd := cmd.Run("oc", "get", "group", group, "-o", "jsonpath={.users[*]}")
	groupExists := getUsersCmd.ExitCode == 0
	if groupExists {
		out := strings.TrimSpace(getUsersCmd.Stdout())
		if out != "" {
			oldUsers = strings.Fields(out)
		}
	}

	usersJSON, err := json.Marshal(users)
	Expect(err).NotTo(HaveOccurred(), "failed to marshal group users")
	patch := fmt.Sprintf("{\"users\":%s}", string(usersJSON))

	createRes := cmd.Run("oc", "adm", "groups", "new", group)
	if createRes.ExitCode != 0 {
		stderr := strings.ToLower(createRes.Stderr())
		Expect(strings.Contains(stderr, "already exists") || strings.Contains(stderr, "alreadyexists")).To(
			BeTrue(), "failed to create group %s: %s", group, createRes.Stderr())
	}

	out := strings.TrimSpace(cmd.MustSucceed("oc", "patch", "group", group, "--type=merge", "-p", patch, "-o", "jsonpath={.users[*]}").Stdout())
	actual := []string{}
	if out != "" {
		actual = strings.Fields(out)
	}

	expected := append([]string{}, users...)
	sort.Strings(expected)
	sort.Strings(actual)
	Expect(strings.Join(actual, ",")).To(Equal(strings.Join(expected, ",")),
		"group %s membership mismatch: expected [%s], got [%s]", group, strings.Join(expected, " "), strings.Join(actual, " "))

	oldSorted := append([]string{}, oldUsers...)
	sort.Strings(oldSorted)
	if !groupExists || strings.Join(oldSorted, ",") != strings.Join(actual, ",") {
		union := append([]string{}, oldUsers...)
		union = append(union, actual...)
		markUsersAuthDirty(union)
	}
}

// DeleteGroup removes an OpenShift Group; ignores not-found errors.
func DeleteGroup(group string) {
	cmd.Run("oc", "delete", "group", group, "--ignore-not-found")
}

// CreateApprovalPipelineRun creates a PipelineRun with an ApprovalTask and waits for the task name.
// Returns (pipelineRunName, approvalTaskName).
func CreateApprovalPipelineRun(cs *clients.Clients, id, description string, approvers []string, required int, timeout, namespace string) (string, string) {
	Expect(approvers).NotTo(BeEmpty(), "approvers list must not be empty")
	Expect(required).To(BeNumerically(">", 0), "numberOfApprovalsRequired must be > 0")

	var approverLines strings.Builder
	for _, a := range approvers {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		approverLines.WriteString("- ")
		approverLines.WriteString(a)
		approverLines.WriteString("\n")
	}
	approversRaw := strings.TrimSuffix(approverLines.String(), "\n")
	Expect(strings.TrimSpace(approversRaw)).NotTo(BeEmpty(), "approvers list is empty after trimming")
	approversYAML := strings.ReplaceAll(approversRaw, "\n", "\n              ")

	idLower := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(id), " ", "-"))
	idLower = strings.ReplaceAll(idLower, "_", "-")
	genName := fmt.Sprintf("approva-grp-plr-%s-", idLower)

	prYAML := fmt.Sprintf(`apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: %s
  namespace: %s
spec:
  pipelineSpec:
    tasks:
      - name: wait
        timeout: %s
        taskRef:
          apiVersion: openshift-pipelines.org/v1alpha1
          kind: ApprovalTask
        params:
          - name: approvers
            value:
              %s
          - name: numberOfApprovalsRequired
            value: "%d"
          - name: description
            value: "%s"
`, genName, namespace, timeout, approversYAML, required, description)

	prName := strings.TrimSpace(cmd.MustSucceedWithStdin(strings.NewReader(prYAML), "oc", "create", "-n", namespace, "-f", "-", "-o", "jsonpath={.metadata.name}").Stdout())
	Expect(prName).NotTo(BeEmpty(), "failed to create PipelineRun: got empty name")

	taskName := WaitForSingleApprovalTaskName(cs, prName, namespace, 2*time.Minute)
	log.Printf("[MAG] %s: created PipelineRun=%s ApprovalTask=%s", id, prName, taskName)
	return prName, taskName
}

// WaitForSingleApprovalTaskName polls until an ApprovalTask appears and returns its name.
func WaitForSingleApprovalTaskName(cs *clients.Clients, prName, namespace string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var picked *atv1alpha1.ApprovalTask
	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		list, err := cs.ApprovalTask.List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		if len(list.Items) == 0 {
			return false, nil
		}
		sort.SliceStable(list.Items, func(i, j int) bool {
			return list.Items[i].CreationTimestamp.After(list.Items[j].CreationTimestamp.Time)
		})
		picked = &list.Items[0]
		return true, nil
	})
	Expect(err).NotTo(HaveOccurred(), "timed out waiting for ApprovalTask for pipelinerun %s in namespace %s", prName, namespace)
	Expect(picked).NotTo(BeNil(), "no ApprovalTask found for pipelinerun %s", prName)
	return picked.Name
}

// WaitForApprovalTaskState polls until the named ApprovalTask reaches the expected state.
func WaitForApprovalTaskState(cs *clients.Clients, task, expectedState string, timeout time.Duration) {
	exp := strings.ToLower(expectedState)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		at, err := cs.ApprovalTask.Get(ctx, task, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return strings.ToLower(at.Status.State) == exp, nil
	})
	Expect(err).NotTo(HaveOccurred(), "timed out waiting for ApprovalTask %s to reach state %s", task, expectedState)
}

// WaitForApprovalTaskMessageContains polls until the ApprovalTask contains the expected message.
func WaitForApprovalTaskMessageContains(cs *clients.Clients, task, text string, timeout time.Duration) {
	text = strings.TrimSpace(text)
	Expect(text).NotTo(BeEmpty(), "message text must not be empty")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var last *atv1alpha1.ApprovalTask
	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		at, err := cs.ApprovalTask.Get(ctx, task, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		last = at
		return approvalTaskMessageContains(at, text), nil
	})
	lastState := ""
	if last != nil {
		lastState = last.Status.State
	}
	Expect(err).NotTo(HaveOccurred(),
		"timed out waiting for ApprovalTask %s to contain message %q; last status=%q", task, text, lastState)
}

// WaitForAndAssertApprovalTaskListState polls until the ApprovalTask matches all expected counters and status.
func WaitForAndAssertApprovalTaskListState(cs *clients.Clients, task string, expectedNum, expectedPending, expectedRejected int, expectedStatus string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var last *atv1alpha1.ApprovalTask
	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		at, err := cs.ApprovalTask.Get(ctx, task, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		last = at
		return at.Spec.NumberOfApprovalsRequired == expectedNum &&
			pendingApprovals(at) == expectedPending &&
			rejectedCount(at) == expectedRejected &&
			stateHuman(at) == expectedStatus, nil
	})

	if err != nil && last != nil {
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("approvaltask %s list-state mismatch: expected num=%d pending=%d rejected=%d status=%s; got num=%d pending=%d rejected=%d status=%s",
				task, expectedNum, expectedPending, expectedRejected, expectedStatus,
				last.Spec.NumberOfApprovalsRequired, pendingApprovals(last), rejectedCount(last), stateHuman(last)))
	} else {
		Expect(err).NotTo(HaveOccurred(), "failed to read ApprovalTask %s for state assertion", task)
	}
}

func approvalTaskMessageContains(at *atv1alpha1.ApprovalTask, text string) bool {
	if at == nil || text == "" {
		return false
	}
	for _, a := range at.Spec.Approvers {
		if strings.Contains(a.Message, text) {
			return true
		}
	}
	for _, r := range at.Status.ApproversResponse {
		if strings.Contains(r.Message, text) {
			return true
		}
		for _, m := range r.GroupMembers {
			if strings.Contains(m.Message, text) {
				return true
			}
		}
	}
	return false
}

// pendingApprovals matches the CLI calculation (see manual-approval-gate/pkg/cli/cmd/list).
func pendingApprovals(at *atv1alpha1.ApprovalTask) int {
	respondedUsers := make(map[string]bool)
	for _, approver := range at.Status.ApproversResponse {
		switch atv1alpha1.DefaultedApproverType(approver.Type) {
		case "User":
			respondedUsers[approver.Name] = true
		case "Group":
			for _, member := range approver.GroupMembers {
				if member.Response == "approved" || member.Response == "rejected" {
					respondedUsers[member.Name] = true
				}
			}
		}
	}
	return at.Spec.NumberOfApprovalsRequired - len(respondedUsers)
}

// rejectedCount matches the CLI calculation (see manual-approval-gate/pkg/cli/cmd/list).
func rejectedCount(at *atv1alpha1.ApprovalTask) int {
	count := 0
	rejectedUsers := make(map[string]bool)
	for _, approver := range at.Status.ApproversResponse {
		if atv1alpha1.DefaultedApproverType(approver.Type) == "User" && approver.Response == "rejected" {
			if !rejectedUsers[approver.Name] {
				rejectedUsers[approver.Name] = true
				count++
			}
		} else if atv1alpha1.DefaultedApproverType(approver.Type) == "Group" {
			for _, member := range approver.GroupMembers {
				if member.Response == "rejected" {
					if !rejectedUsers[member.Name] {
						rejectedUsers[member.Name] = true
						count++
					}
				}
			}
		}
	}
	return count
}

func stateHuman(at *atv1alpha1.ApprovalTask) string {
	switch at.Status.State {
	case "approved":
		return "Approved"
	case "rejected":
		return "Rejected"
	case "pending":
		return "Pending"
	default:
		return at.Status.State
	}
}

// ── Per-User Action Helpers ───────────────────────────────────────────────────

func ensureMAGAPIServer() string {
	magAPIServerOnce.Do(func() {
		api := strings.TrimSpace(cmd.MustSucceed("oc", "whoami", "--show-server").Stdout())
		if api == "" {
			magAPIServerErr = fmt.Errorf("failed to detect cluster API server via `oc whoami --show-server`")
			return
		}
		magAPIServer = api
	})
	Expect(magAPIServerErr).NotTo(HaveOccurred(), "could not detect cluster API server")
	return magAPIServer
}

func ensureUserKubeconfig(user string) string {
	magUserKubeconfigsMu.Lock()
	if v, ok := magUserKubeconfigs[user]; ok && strings.TrimSpace(v) != "" {
		magUserKubeconfigsMu.Unlock()
		if popUserAuthDirty(user) {
			apiServer := ensureMAGAPIServer()
			pass := userPassword(user)
			cmd.MustSucceed("oc", "login", apiServer, "-u", user, "-p", pass, "--kubeconfig", v, "--insecure-skip-tls-verify=true")
		}
		return v
	}
	magUserKubeconfigsMu.Unlock()

	apiServer := ensureMAGAPIServer()
	pass := userPassword(user)

	tmp, err := os.CreateTemp("", fmt.Sprintf("mag-kubeconfig-%s-", user))
	Expect(err).NotTo(HaveOccurred(), "failed to create temp kubeconfig for %s", user)
	_ = tmp.Close()

	kcPath := tmp.Name()
	cmd.MustSucceed("oc", "login", apiServer, "-u", user, "-p", pass, "--kubeconfig", kcPath, "--insecure-skip-tls-verify=true")

	magUserKubeconfigsMu.Lock()
	magUserKubeconfigs[user] = kcPath
	magUserKubeconfigsMu.Unlock()
	_ = popUserAuthDirty(user)
	return kcPath
}

// CleanupUserKubeconfigs removes temp kubeconfig files created for per-user logins.
func CleanupUserKubeconfigs() {
	magUserKubeconfigsMu.Lock()
	defer magUserKubeconfigsMu.Unlock()
	for user, path := range magUserKubeconfigs {
		if strings.TrimSpace(path) != "" {
			_ = os.Remove(path)
		}
		delete(magUserKubeconfigs, user)
	}
	magUserAuthDirtyMu.Lock()
	magUserAuthDirty = map[string]bool{}
	magUserAuthDirtyMu.Unlock()
}

// ApproveApprovalTaskAsUser approves the task as the given user.
func ApproveApprovalTaskAsUser(user, task, namespace, message string) {
	kc := ensureUserKubeconfig(user)
	args := []string{"opc", "approvaltask", "approve", task, "-n", namespace}
	if strings.TrimSpace(message) != "" {
		args = append(args, "-m", message)
	}
	cmd.MustSucceedWithEnv([]string{"KUBECONFIG=" + kc}, args...)
}

// RejectApprovalTaskAsUser rejects the task as the given user.
func RejectApprovalTaskAsUser(user, task, namespace, message string) {
	kc := ensureUserKubeconfig(user)
	args := []string{"opc", "approvaltask", "reject", task, "-n", namespace}
	if strings.TrimSpace(message) != "" {
		args = append(args, "-m", message)
	}
	cmd.MustSucceedWithEnv([]string{"KUBECONFIG=" + kc}, args...)
}

// ApproveApprovalTaskExpectFailAsUser asserts that the approval attempt fails (e.g. non-member).
func ApproveApprovalTaskExpectFailAsUser(user, task, namespace, message string) {
	kc := ensureUserKubeconfig(user)
	args := []string{"opc", "approvaltask", "approve", task, "-n", namespace}
	if strings.TrimSpace(message) != "" {
		args = append(args, "-m", message)
	}
	res := cmd.RunWithEnv([]string{"KUBECONFIG=" + kc}, args...)
	Expect(res.ExitCode).NotTo(Equal(0),
		"expected approval by %s on %s to fail, but it succeeded", user, task)
}

// ApproveApprovalTaskAllowFinalStateAsUser approves but tolerates "already reached final state" errors.
func ApproveApprovalTaskAllowFinalStateAsUser(user, task, namespace, message string) {
	kc := ensureUserKubeconfig(user)
	args := []string{"opc", "approvaltask", "approve", task, "-n", namespace}
	if strings.TrimSpace(message) != "" {
		args = append(args, "-m", message)
	}
	res := cmd.RunWithEnv([]string{"KUBECONFIG=" + kc}, args...)
	if res.ExitCode == 0 {
		return
	}
	out := strings.ToLower(res.Stdout() + "\n" + res.Stderr())
	Expect(strings.Contains(out, "already reached") && strings.Contains(out, "final state")).To(
		BeTrue(), "unexpected approval failure for %s on %s: %s", user, task, res.Stderr())
}

type approvalTaskActionFn func(user, task, namespace, message string)

var approvalTaskActionDispatch = map[string]approvalTaskActionFn{
	"approve":                   ApproveApprovalTaskAsUser,
	"reject":                    RejectApprovalTaskAsUser,
	"approve-expect-fail":       ApproveApprovalTaskExpectFailAsUser,
	"approve-allow-final-state": ApproveApprovalTaskAllowFinalStateAsUser,
}

// PerformApprovalTaskActionAsUser dispatches the named action for the given user on the task.
func PerformApprovalTaskActionAsUser(user, action, task, namespace, message string) {
	a := strings.ToLower(strings.TrimSpace(action))
	Expect(a).NotTo(BeEmpty(), "approval task action must not be empty")
	fn, ok := approvalTaskActionDispatch[a]
	Expect(ok).To(BeTrue(), "unsupported approval gate action: %s", action)
	fn(user, task, namespace, message)
}
