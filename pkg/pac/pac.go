// Package pac provides helpers for interacting with Pipelines-as-Code resources.
package pac

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot import is idiomatic for Ginkgo
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/cli"
	pacgenerate "github.com/openshift-pipelines/pipelines-as-code/pkg/cmd/tknpac/generate"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/git"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/params/info"
	gitlab "github.com/xanzy/go-gitlab"
	yaml "gopkg.in/yaml.v2"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/clients"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/config"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/k8s"
	occmd "github.com/openshift-pipelines/release-tests-ginkgo/pkg/oc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/opc"
	"github.com/openshift-pipelines/release-tests-ginkgo/pkg/pipelines"
)

var oc = occmd.OC{}

const (
	initialBackoffDuration   = 5 * time.Second
	maxRetriesForkProject    = 5
	maxRetriesPipelineStatus = 10
	targetURL                = "http://pipelines-as-code-controller.openshift-pipelines:8080"
	webhookConfigName        = "gitlab-webhook-config"
	pullRequestFileName      = "/tmp/pull_request.yaml"
	pushFileName             = "/tmp/push.yaml"
)

// client is the package-level GitLab client. Within Ordered containers
// this is safe since PAC tests run serially via SetGitLabClient.
var client *gitlab.Client

// projectURL holds the web URL of the forked project, set during SetupGitLabProject.
var projectURL string

// SetGitLabClient sets the package-level GitLab client.
func SetGitLabClient(c *gitlab.Client) {
	client = c
}

// InitGitLabClient creates a GitLab client from environment variables and ensures the
// webhook secret exists in the target namespace.
func InitGitLabClient(namespace string) *gitlab.Client {
	tokenSecretData := os.Getenv("GITLAB_TOKEN")
	webhookSecretData := os.Getenv("GITLAB_WEBHOOK_TOKEN")
	if tokenSecretData == "" || webhookSecretData == "" {
		Fail("token for authorization to the GitLab repository was not exported as a system variable")
	} else {
		if !oc.SecretExists(webhookConfigName, namespace) {
			oc.CreateSecretForWebhook(tokenSecretData, webhookSecretData, namespace)
		} else {
			log.Printf("Secret %q already exists", webhookConfigName)
		}
	}
	c, err := gitlab.NewClient(tokenSecretData)
	if err != nil {
		Fail(fmt.Sprintf("failed to initialize GitLab client: %v", err))
	}

	return c
}

// getNewSmeeURL retrieves a new Smee URL from smee.io.
func getNewSmeeURL() (string, error) {
	curlCommand := `curl -Ls -o /dev/null -w %{url_effective} https://smee.io/new`
	cmd := exec.Command("sh", "-c", curlCommand)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to create SmeeURL: %w", err)
	}
	smeeURL := strings.TrimSpace(string(output))
	if smeeURL == "" {
		return "", fmt.Errorf("failed to retrieve Smee URL: no URL found")
	}
	return smeeURL, nil
}

// createSmeeDeployment creates a gosmee-client deployment in the namespace.
func createSmeeDeployment(c *clients.Clients, namespace, smeeURL string) error {
	kc := c.KubeClient.Kube
	deploymentsClient := kc.AppsV1().Deployments(namespace)
	existing, err := deploymentsClient.Get(context.TODO(), "gosmee-client", metav1.GetOptions{})
	if err == nil && existing != nil {
		log.Printf("Deployment %q already present in %q; leaving as-is", "gosmee-client", namespace)
		return nil
	}

	replicas := int32(1)
	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gosmee-client",
			Labels: map[string]string{
				"app": "gosmee-client",
			},
		},
		Spec: v1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "gosmee-client"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "gosmee-client"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "gosmee-client",
							Image: "ghcr.io/chmouel/gosmee:latest",
							Command: []string{
								"gosmee",
								"client",
								smeeURL,
								targetURL,
							},
							Env: []corev1.EnvVar{
								{Name: "SMEE_URL", Value: smeeURL},
								{Name: "TARGET_URL", Value: targetURL},
							},
						},
					},
				},
			},
		},
	}

	result, err := deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}
	log.Printf("Created deployment %q in namespace %q.\n", result.GetObjectMeta().GetName(), namespace)
	return nil
}

// SetupSmeeDeployment creates a Smee deployment and returns the Smee URL.
// Replaces the Gauge version that used store.PutScenarioData.
func SetupSmeeDeployment(c *clients.Clients, namespace string) (smeeURL string, err error) {
	smeeURL, err = getNewSmeeURL()
	if err != nil {
		return "", fmt.Errorf("failed to get a new Smee URL: %w", err)
	}

	if err = createSmeeDeployment(c, namespace, smeeURL); err != nil {
		return "", fmt.Errorf("failed to create deployment: %w", err)
	}

	return smeeURL, nil
}

// forkProject forks a GitLab project into the specified group namespace.
func forkProject(projectID, targetNamespace string) (*gitlab.Project, error) {
	for i := 0; i < maxRetriesForkProject; i++ {
		projectName := fmt.Sprintf("release-tests-fork-%08d", time.Now().UnixNano()%1e8)
		project, _, err := client.Projects.ForkProject(projectID, &gitlab.ForkProjectOptions{
			Namespace: &targetNamespace,
			Name:      &projectName,
			Path:      &projectName,
		})
		if err == nil {
			projectURL = project.WebURL
			return project, nil
		}
		log.Printf("Retry %d: failed to fork project: %v", i+1, err)
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return nil, fmt.Errorf("failed to fork project after %d attempts", maxRetriesForkProject)
}

// addWebhook adds a webhook to the forked project.
func addWebhook(projectID int, webhookURL, token string) error {
	pushEvents := true
	mergeRequestsEvents := true
	noteEvents := true
	tagPushEvents := true

	hookOptions := &gitlab.AddProjectHookOptions{
		URL:                 &webhookURL,
		PushEvents:          &pushEvents,
		MergeRequestsEvents: &mergeRequestsEvents,
		NoteEvents:          &noteEvents,
		TagPushEvents:       &tagPushEvents,
		Token:               &token,
	}

	_, _, err := client.Projects.AddProjectHook(projectID, hookOptions)
	if err != nil {
		return fmt.Errorf("failed to add webhook: %w", err)
	}
	return nil
}

// createNewRepository creates a PAC Repository CR in the namespace.
func createNewRepository(c *clients.Clients, projectName, targetGroupNamespace, namespace string) error {
	repo := &pacv1alpha1.Repository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "pipelinesascode.tekton.dev/v1alpha1",
			Kind:       "Repository",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName,
			Namespace: namespace,
		},
		Spec: pacv1alpha1.RepositorySpec{
			URL: fmt.Sprintf("https://gitlab.com/%s/%s", targetGroupNamespace, projectName),
			GitProvider: &pacv1alpha1.GitProvider{
				URL: "https://gitlab.com",
				Secret: &pacv1alpha1.Secret{
					Name: webhookConfigName,
					Key:  "provider.token",
				},
				WebhookSecret: &pacv1alpha1.Secret{
					Name: webhookConfigName,
					Key:  "webhook.secret",
				},
			},
		},
	}

	created, err := c.PacClientset.Repositories(namespace).Create(context.Background(), repo, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	log.Printf("Repository %q created successfully in namespace %q", created.GetName(), created.GetNamespace())
	return nil
}

// addLabelToProject adds a label to a GitLab project if it doesn't already exist.
func addLabelToProject(projectID int, labelName, color, description string) error {
	labels, _, err := client.Labels.ListLabels(projectID, &gitlab.ListLabelsOptions{})
	if err != nil {
		return fmt.Errorf("failed to fetch project labels: %w", err)
	}
	for _, label := range labels {
		if label.Name == labelName {
			log.Printf("Label %q already exists in project ID %d\n", labelName, projectID)
			return nil
		}
	}

	_, _, err = client.Labels.CreateLabel(projectID, &gitlab.CreateLabelOptions{
		Name:        gitlab.Ptr(labelName),
		Color:       gitlab.Ptr(color),
		Description: gitlab.Ptr(description),
	})
	if err != nil {
		return fmt.Errorf("failed to create label %q: %w", labelName, err)
	}
	log.Printf("Successfully added label %q to project ID %d\n", labelName, projectID)
	return nil
}

// SetupGitLabProject forks the project, adds webhook, and creates the Repository CR.
// Returns the forked project and its ID. Replaces the Gauge version that used store.*.
func SetupGitLabProject(c *clients.Clients, namespace, smeeURL string) (*gitlab.Project, error) {
	gitlabGroupNamespace := os.Getenv("GITLAB_GROUP_NAMESPACE")
	projectIDOrPath := os.Getenv("GITLAB_PROJECT_ID")

	if gitlabGroupNamespace == "" || projectIDOrPath == "" {
		return nil, fmt.Errorf("GITLAB_GROUP_NAMESPACE and GITLAB_PROJECT_ID must be set")
	}

	webhookToken := os.Getenv("GITLAB_WEBHOOK_TOKEN")

	project, err := forkProject(projectIDOrPath, gitlabGroupNamespace)
	if err != nil {
		return nil, fmt.Errorf("error during project forking: %w", err)
	}

	err = addWebhook(project.ID, smeeURL, webhookToken)
	if err != nil {
		return nil, fmt.Errorf("failed to add webhook: %w", err)
	}

	err = createNewRepository(c, project.Name, gitlabGroupNamespace, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	return project, nil
}

// AddComment adds a comment to the specified merge request.
func AddComment(projectID, mrID int, comment string) error {
	opts := &gitlab.CreateMergeRequestNoteOptions{
		Body: gitlab.Ptr(comment),
	}

	_, _, err := client.Notes.CreateMergeRequestNote(projectID, mrID, opts)
	if err != nil {
		return fmt.Errorf("failed to add comment to MR %d in project %d: %w", mrID, projectID, err)
	}
	log.Printf("Successfully added comment %s to merge request %d\n", comment, mrID)
	return nil
}

// AddLabel adds a label to the GitLab project and applies it to the merge request.
func AddLabel(projectID, mrID int, label, color, description string) error {
	err := addLabelToProject(projectID, label, color, description)
	if err != nil {
		return fmt.Errorf("failed to add label to project: %w", err)
	}

	addLabels := gitlab.LabelOptions{label}

	_, _, err = client.MergeRequests.UpdateMergeRequest(projectID, mrID, &gitlab.UpdateMergeRequestOptions{
		AddLabels: &addLabels,
	})
	if err != nil {
		return fmt.Errorf("failed to update merge request with label %q: %w", label, err)
	}
	log.Printf("Successfully added label %s to merge request %d\n", label, mrID)
	return nil
}

// createBranch creates a new branch from main.
func createBranch(projectID int, branchName string) error {
	_, _, err := client.Branches.CreateBranch(projectID, &gitlab.CreateBranchOptions{
		Branch: gitlab.Ptr(branchName),
		Ref:    gitlab.Ptr("main"),
	})
	return err
}

// createPacGenerateOpts sets up the PAC generate options for the given event type and branch.
func createPacGenerateOpts(eventType, branch, fileName string) *pacgenerate.Opts {
	opts := pacgenerate.MakeOpts()

	opts.Event = &info.Event{
		EventType:  eventType,
		BaseBranch: branch,
	}
	opts.GitInfo = &git.Info{
		URL:    projectURL,
		Branch: branch,
	}
	var outputBuffer bytes.Buffer
	opts.IOStreams = &cli.IOStreams{
		Out:    &outputBuffer,
		ErrOut: os.Stderr,
		In:     os.Stdin,
	}
	opts.FileName = fileName

	return opts
}

// generatePipelineRun generates a sample PipelineRun YAML file.
func generatePipelineRun(eventType, branch, fileName string) error {
	if _, err := os.Stat(fileName); err == nil {
		_ = os.Remove(fileName)
	}
	opts := createPacGenerateOpts(eventType, branch, fileName)
	if err := pacgenerate.Generate(opts, true); err != nil {
		return fmt.Errorf("failed to generate PipelineRun: %w", err)
	}
	return nil
}

// validateYAML validates that the provided bytes are valid YAML.
func validateYAML(yamlContent []byte) error {
	var content map[string]any
	if err := yaml.Unmarshal(yamlContent, &content); err != nil {
		return fmt.Errorf("invalid YAML format: %w", err)
	}
	return nil
}

// GeneratePipelineRunYaml generates a PipelineRun YAML for the given event type and branch.
// The generated file is stored in /tmp/{pull_request,push}.yaml.
func GeneratePipelineRunYaml(eventType, branch string) error {
	fileName := eventType + ".yaml"

	if err := generatePipelineRun(eventType, branch, fileName); err != nil {
		return fmt.Errorf("failed to generate pipelinerun: %w", err)
	}

	fileContent, err := os.ReadFile(filepath.Clean(fileName))
	if err != nil {
		return fmt.Errorf("could not read file %s: %w", fileName, err)
	}

	if err := validateYAML(fileContent); err != nil {
		return fmt.Errorf("invalid YAML content: %w", err)
	}

	var destPath string
	switch eventType {
	case "pull_request":
		destPath = pullRequestFileName
	case "push":
		destPath = pushFileName
	default:
		return fmt.Errorf("unknown eventType: %s", eventType)
	}
	if err := os.WriteFile(destPath, fileContent, 0600); err != nil { //nolint:gosec // G703: path is sanitized before this call
		return fmt.Errorf("failed to write %s: %w", destPath, err)
	}
	return nil
}

// UpdateAnnotation updates the specified annotation in the pull-request.yaml file.
// Returns the updated YAML content.
func UpdateAnnotation(annotationKey, annotationValue string) (string, error) {
	fileName := pullRequestFileName
	data, err := os.ReadFile(filepath.Clean(fileName))
	if err != nil {
		return "", fmt.Errorf("failed to read YAML file: %w", err)
	}

	var content map[string]any
	if err := yaml.Unmarshal(data, &content); err != nil {
		return "", fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	meta := content["metadata"].(map[any]any)
	anns := meta["annotations"].(map[any]any)

	if currValue, exists := anns[annotationKey].(string); exists {
		anns[annotationKey] = currValue + " " + annotationValue
	} else {
		anns[annotationKey] = annotationValue
	}

	out, err := yaml.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(fileName, out, 0600); err != nil {
		return "", fmt.Errorf("failed to write YAML file: %w", err)
	}

	if err := validateYAML(out); err != nil {
		return "", fmt.Errorf("invalid YAML content: %w", err)
	}

	log.Println("Annotation updated successfully")
	return string(out), nil
}

// createCommit commits a single file (pull_request or push) to the given branch.
func createCommit(projectID int, branch, commitMessage, eventType string) error {
	action := gitlab.FileCreate
	var actions []*gitlab.CommitActionOptions

	switch eventType {
	case "pull_request":
		data, err := os.ReadFile(pullRequestFileName)
		if err != nil {
			return fmt.Errorf("read PR file: %w", err)
		}
		actions = append(actions, &gitlab.CommitActionOptions{
			Action:   &action,
			FilePath: gitlab.Ptr(".tekton/pull-request.yaml"),
			Content:  gitlab.Ptr(string(data)),
		})
	case "push":
		data, err := os.ReadFile(pushFileName)
		if err != nil {
			return fmt.Errorf("read push file: %w", err)
		}
		actions = append(actions, &gitlab.CommitActionOptions{
			Action:   &action,
			FilePath: gitlab.Ptr(".tekton/push.yaml"),
			Content:  gitlab.Ptr(string(data)),
		})
	default:
		return fmt.Errorf("unknown eventType %q", eventType)
	}

	commitOpts := &gitlab.CreateCommitOptions{
		Branch:        &branch,
		CommitMessage: &commitMessage,
		Actions:       actions,
	}
	if _, _, err := client.Commits.CreateCommit(projectID, commitOpts); err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}
	return nil
}

// createMergeRequest creates a merge request on the forked project.
func createMergeRequest(projectID int, sourceBranch, targetBranch, title string) (string, error) {
	mrOptions := &gitlab.CreateMergeRequestOptions{
		SourceBranch: &sourceBranch,
		TargetBranch: &targetBranch,
		Title:        &title,
	}
	mr, _, err := client.MergeRequests.CreateMergeRequest(projectID, mrOptions)
	if err != nil {
		return "", err
	}
	return mr.WebURL, nil
}

// extractMergeRequestID extracts the MR ID from a merge request URL.
func extractMergeRequestID(mrURL string) (int, error) {
	parsedURL, err := url.Parse(mrURL)
	if err != nil {
		return 0, fmt.Errorf("failed to parse merge request URL: %w", err)
	}
	segments := strings.Split(parsedURL.Path, "/")
	mrIDStr := segments[len(segments)-1]
	mrID, err := strconv.Atoi(mrIDStr)
	if err != nil {
		return 0, fmt.Errorf("failed to convert MR ID to integer: %w", err)
	}
	return mrID, nil
}

// isTerminalStatus returns true if the pipeline status is a terminal state.
func isTerminalStatus(status string) bool {
	return status == "success" || status == "failed" || status == "canceled"
}

// checkPipelineStatus waits for the pipeline status of a merge request to reach a terminal state.
func checkPipelineStatus(projectID, mergeRequestID int) error {
	retryCount := 0
	delay := initialBackoffDuration
	const maxDelay = 60 * time.Second

	for {
		pipelinesList, _, err := client.MergeRequests.ListMergeRequestPipelines(projectID, mergeRequestID)
		if err != nil {
			return fmt.Errorf("failed to list merge request pipelines: %w", err)
		}

		if len(pipelinesList) == 0 {
			if retryCount >= maxRetriesPipelineStatus {
				log.Printf("No pipelines found for the MR id %d after %d retries\n", mergeRequestID, maxRetriesPipelineStatus)
				return nil
			}
			log.Println("No pipelines found, retrying...")
			time.Sleep(delay)
			retryCount++
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}

		latestPipeline := pipelinesList[0]
		if isTerminalStatus(latestPipeline.Status) {
			log.Printf("Latest pipeline status for MR #%d: %s\n", mergeRequestID, latestPipeline.Status)
			return nil
		}
		log.Println("waiting for Pipeline status to be updated...")
		time.Sleep(10 * time.Second)
	}
}

// ConfigurePreviewChanges creates a preview branch, commits pipeline files, and creates a MR.
// Returns the MR ID.
func ConfigurePreviewChanges(projectID int) (mrID int, err error) {
	gen := func(n int) (string, error) {
		const abc = "abcdefghijklmnopqrstuvwxyz0123456789"
		out := make([]byte, n)
		for i := range out {
			k, err := rand.Int(rand.Reader, big.NewInt(int64(len(abc))))
			if err != nil {
				return "", err
			}
			out[i] = abc[int(k.Int64())]
		}
		return string(out), nil
	}
	branchExists := func(name string) bool {
		_, resp, err := client.Branches.GetBranch(projectID, name)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				return false
			}
			Fail(fmt.Sprintf("GetBranch(%q): %v", name, err))
		}
		return true
	}

	var branchName string
	for range 10 {
		suf, err := gen(8)
		if err != nil {
			return 0, fmt.Errorf("failed to generate branch suffix: %w", err)
		}
		n := "preview-" + suf
		if !branchExists(n) {
			branchName = n
			break
		}
	}
	if branchName == "" {
		branchName = "preview-branch-" + strings.ToLower(strconv.FormatInt(time.Now().UnixNano(), 36))[:8]
	}

	if err := createBranch(projectID, branchName); err != nil {
		return 0, fmt.Errorf("createBranch %q: %w", branchName, err)
	}

	prExists := false
	pushExists := false
	if _, err := os.Stat(pullRequestFileName); err == nil {
		prExists = true
	}
	if _, err := os.Stat(pushFileName); err == nil {
		pushExists = true
	}

	if prExists && pushExists {
		action := gitlab.FileCreate
		prData, err := os.ReadFile(pullRequestFileName)
		if err != nil {
			return 0, fmt.Errorf("read PR file: %w", err)
		}
		pushData, err := os.ReadFile(pushFileName)
		if err != nil {
			return 0, fmt.Errorf("read push file: %w", err)
		}
		msg := "ci(pac): add push & pull_request files"
		commitOpts := &gitlab.CreateCommitOptions{
			Branch:        &branchName,
			CommitMessage: &msg,
			Actions: []*gitlab.CommitActionOptions{
				{Action: &action, FilePath: gitlab.Ptr(".tekton/pull-request.yaml"), Content: gitlab.Ptr(string(prData))},
				{Action: &action, FilePath: gitlab.Ptr(".tekton/push.yaml"), Content: gitlab.Ptr(string(pushData))},
			},
		}
		if _, _, err := client.Commits.CreateCommit(projectID, commitOpts); err != nil {
			return 0, fmt.Errorf("commit both: %w", err)
		}
	} else if prExists {
		if err := createCommit(projectID, branchName, "ci(pac): add pull_request file", "pull_request"); err != nil {
			return 0, fmt.Errorf("commit pull_request: %w", err)
		}
	} else if pushExists {
		if err := createCommit(projectID, branchName, "ci(pac): add push file", "push"); err != nil {
			return 0, fmt.Errorf("commit push: %w", err)
		}
	} else {
		return 0, fmt.Errorf("no pipeline files found to commit in /tmp")
	}

	mrURL, err := createMergeRequest(projectID, branchName, "main", "Add preview changes for feature")
	if err != nil {
		return 0, fmt.Errorf("createMergeRequest: %w", err)
	}
	log.Printf("Merge Request Created: %s\n", mrURL)

	mrID, err = extractMergeRequestID(mrURL)
	if err != nil {
		return 0, fmt.Errorf("extract MR ID: %w", err)
	}
	return mrID, nil
}

// repoFileExists checks if a file exists at the given path on the specified branch.
func repoFileExists(projectID int, branch, path string) (bool, error) {
	f, resp, err := client.RepositoryFiles.GetFile(projectID, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(branch)})
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("GetFile failed for %s on %s: %w", path, branch, err)
	}
	return f != nil, nil
}

// TriggerPushOnForkMain commits push.yaml to the main branch along with a trigger file
// to trigger a push pipeline event.
func TriggerPushOnForkMain(projectID int) error {
	data, err := os.ReadFile("/tmp/push.yaml")
	if err != nil {
		return fmt.Errorf("failed to read /tmp/push.yaml: %w", err)
	}
	pushFileContent := string(data)

	branch := "main"
	pushYamlPath := ".tekton/push.yaml"
	triggerPath := fmt.Sprintf("ci/push-trigger-%d.txt", time.Now().Unix())

	exists, err := repoFileExists(projectID, branch, pushYamlPath)
	if err != nil {
		return err
	}

	var actionPushYaml gitlab.FileActionValue
	if exists {
		actionPushYaml = gitlab.FileUpdate
	} else {
		actionPushYaml = gitlab.FileCreate
	}

	createAction := gitlab.FileCreate

	commitMsg := "ci(pac): add push.yaml on main and trigger push pipeline"
	actions := []*gitlab.CommitActionOptions{
		{
			Action:   &actionPushYaml,
			FilePath: gitlab.Ptr(pushYamlPath),
			Content:  gitlab.Ptr(pushFileContent),
		},
		{
			Action:   &createAction,
			FilePath: gitlab.Ptr(triggerPath),
			Content:  gitlab.Ptr("push-trigger"),
		},
	}

	commitOpts := &gitlab.CreateCommitOptions{
		Branch:        &branch,
		CommitMessage: &commitMsg,
		Actions:       actions,
	}

	if _, _, err := client.Commits.CreateCommit(projectID, commitOpts); err != nil {
		return fmt.Errorf("failed to commit push.yaml+trigger to main: %w", err)
	}
	return nil
}

// GetPipelineNameFromMR checks the MR pipeline status, then returns the latest PipelineRun name.
func GetPipelineNameFromMR(c *clients.Clients, namespace string, projectID, mrID int) (string, error) {
	err := checkPipelineStatus(projectID, mrID)
	if err != nil {
		return "", fmt.Errorf("failed to check pipeline status: %w", err)
	}

	pipelineName, err := pipelines.GetLatestPipelinerun(c, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get the latest Pipelinerun: %w", err)
	}
	return pipelineName, nil
}

// GetPushPipelineNameFromMain waits briefly, then returns the latest PipelineRun name.
// Used after a push event where there is no MR pipeline to poll.
func GetPushPipelineNameFromMain(c *clients.Clients, namespace string) (string, error) {
	time.Sleep(10 * time.Second)

	pipelineName, err := pipelines.GetLatestPipelinerun(c, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get the latest Pipelinerun: %w", err)
	}
	return pipelineName, nil
}

// AssertPACInfoInstall verifies that the PAC installation version and namespace match expectations.
func AssertPACInfoInstall() error {
	pacInfo, err := opc.GetOpcPacInfoInstall()
	if err != nil {
		return fmt.Errorf("failed to get pac info: %w", err)
	}

	clusterVersion := pacInfo.PipelinesAsCode.InstallVersion
	expectedVersion := os.Getenv("PAC_VERSION")

	if !strings.Contains(clusterVersion, expectedVersion) ||
		pacInfo.PipelinesAsCode.InstallNamespace != config.TargetNamespace {
		return fmt.Errorf("PAC version %s doesn't match the expected version %s or namespace %s is wrong",
			clusterVersion, expectedVersion, pacInfo.PipelinesAsCode.InstallNamespace)
	}
	return nil
}

// deleteGitlabProject deletes a GitLab project by ID.
func deleteGitlabProject(projectID int) error {
	_, err := client.Projects.DeleteProject(projectID)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	log.Println("Project successfully deleted.")
	return nil
}

// CleanupPAC removes generated YAML files, deletes the forked GitLab project,
// and deletes the Smee deployment.
func CleanupPAC(c *clients.Clients, namespace string, projectID int, smeeDeploymentName string) error {
	// Remove the generated PipelineRun YAML files
	_ = os.Remove(pullRequestFileName)
	_ = os.Remove(pushFileName)

	// Remove Forked Project
	if cleanupErr := deleteGitlabProject(projectID); cleanupErr != nil {
		return fmt.Errorf("cleanup failed: %w", cleanupErr)
	}

	// Delete Smee Deployment
	if err := k8s.DeleteDeployment(c, namespace, smeeDeploymentName); err != nil {
		return fmt.Errorf("failed to Delete Smee Deployment: %w", err)
	}
	return nil
}

// UpdatePushOnTargetBranch updates the pipelinesascode.tekton.dev/on-target-branch annotation
// in the generated push.yaml file.
func UpdatePushOnTargetBranch(target string) error {
	data, err := os.ReadFile(pushFileName)
	if err != nil {
		return fmt.Errorf("failed to read push YAML file %s: %w", pushFileName, err)
	}

	var content map[string]any
	if err := yaml.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("failed to unmarshal push YAML: %w", err)
	}

	meta, ok := content["metadata"].(map[any]any)
	if !ok {
		return fmt.Errorf("push YAML missing metadata section")
	}
	anns, ok := meta["annotations"].(map[any]any)
	if !ok {
		anns = map[any]any{}
		meta["annotations"] = anns
	}

	anns["pipelinesascode.tekton.dev/on-target-branch"] = target

	out, err := yaml.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to marshal updated push YAML: %w", err)
	}

	if err := os.WriteFile(pushFileName, out, 0600); err != nil {
		return fmt.Errorf("failed to write updated push YAML file: %w", err)
	}

	if err := validateYAML(out); err != nil {
		return fmt.Errorf("invalid YAML content after updating on-target-branch: %w", err)
	}

	log.Printf("Updated pipelinesascode.tekton.dev/on-target-branch to %q in %s\n", target, pushFileName)
	return nil
}

// CreateTagOnBranch creates a Git tag on the specified branch in the current GitLab project.
func CreateTagOnBranch(projectID int, tagName, branch string) error {
	opts := &gitlab.CreateTagOptions{
		TagName: gitlab.Ptr(tagName),
		Ref:     gitlab.Ptr(branch),
	}

	if _, _, err := client.Tags.CreateTag(projectID, opts); err != nil {
		return fmt.Errorf("failed to create tag %q on branch %q: %w", tagName, branch, err)
	}
	log.Printf("Successfully created tag %q on branch %q\n", tagName, branch)
	return nil
}

// getCommitSHAForTag resolves a Git tag to its commit SHA in the current GitLab project.
func getCommitSHAForTag(projectID int, tag string) (string, error) {
	t, _, err := client.Tags.GetTag(projectID, tag)
	if err != nil {
		return "", fmt.Errorf("failed to get tag %q: %w", tag, err)
	}
	if t.Commit == nil || t.Commit.ID == "" {
		return "", fmt.Errorf("tag %q has no associated commit", tag)
	}
	return t.Commit.ID, nil
}

// AddCommitCommentOnTag adds a commit comment on the commit referenced by the given tag.
func AddCommitCommentOnTag(projectID int, comment, tag string) error {
	sha, err := getCommitSHAForTag(projectID, tag)
	if err != nil {
		return fmt.Errorf("failed to resolve tag %q to commit: %w", tag, err)
	}

	opts := &gitlab.PostCommitCommentOptions{
		Note: gitlab.Ptr(comment),
	}

	if _, _, err := client.Commits.PostCommitComment(projectID, sha, opts); err != nil {
		return fmt.Errorf("failed to add comment %q on tag %q (commit %s): %w", comment, tag, sha, err)
	}
	log.Printf("Successfully added comment %q on tag %q (commit %s)\n", comment, tag, sha)
	return nil
}

// getPipelineRunNameFromPushYAML reads the generated push.yaml and returns the
// PipelineRun metadata.name defined there.
func getPipelineRunNameFromPushYAML() (string, error) {
	data, err := os.ReadFile(pushFileName)
	if err != nil {
		return "", fmt.Errorf("failed to read push YAML file %s: %w", pushFileName, err)
	}

	var content map[string]any
	if err := yaml.Unmarshal(data, &content); err != nil {
		return "", fmt.Errorf("failed to unmarshal push YAML: %w", err)
	}

	meta, ok := content["metadata"].(map[any]any)
	if !ok {
		return "", fmt.Errorf("push YAML missing metadata section")
	}
	nameVal, ok := meta["name"].(string)
	if !ok || nameVal == "" {
		return "", fmt.Errorf("push YAML missing or invalid metadata.name")
	}
	log.Printf("PipelineRun name from push.yaml: %s\n", nameVal)
	return nameVal, nil
}

// AddTestCommentForLatestPipelineRunOnTag adds a /test comment for the latest PipelineRun on the given tag.
func AddTestCommentForLatestPipelineRunOnTag(projectID int, tag string) error {
	prName, err := getPipelineRunNameFromPushYAML()
	if err != nil {
		return fmt.Errorf("failed to get PipelineRun name: %w", err)
	}
	comment := fmt.Sprintf("/test %s tag:%s", prName, tag)
	return AddCommitCommentOnTag(projectID, comment, tag)
}

// AddCancelCommentForLatestPipelineRunOnTag adds a /cancel comment for the latest PipelineRun on the given tag.
func AddCancelCommentForLatestPipelineRunOnTag(projectID int, tag string) error {
	prName, err := getPipelineRunNameFromPushYAML()
	if err != nil {
		return fmt.Errorf("failed to get PipelineRun name: %w", err)
	}
	comment := fmt.Sprintf("/cancel %s tag:%s", prName, tag)
	return AddCommitCommentOnTag(projectID, comment, tag)
}

// AssertNumberOfPipelineruns verifies that the expected number of PipelineRuns exist in the
// namespace within the given timeout (in seconds).
func AssertNumberOfPipelineruns(c *clients.Clients, _ string, expectedCount, timeoutSeconds int) error {
	log.Printf("Verifying if %d pipelineruns are present", expectedCount)
	ctx, cancel := context.WithTimeout(c.Ctx, time.Second*time.Duration(timeoutSeconds))
	defer cancel()

	ticker := time.NewTicker(config.APIRetry)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			prlist, _ := c.PipelineRunClient.List(c.Ctx, metav1.ListOptions{})
			actual := 0
			if prlist != nil {
				actual = len(prlist.Items)
			}
			return fmt.Errorf("expected %d pipelineruns but found %d (timeout after %ds)", expectedCount, actual, timeoutSeconds)
		case <-ticker.C:
			prlist, err := c.PipelineRunClient.List(c.Ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}
			if len(prlist.Items) == expectedCount {
				return nil
			}
		}
	}
}
