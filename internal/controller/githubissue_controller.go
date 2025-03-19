/*
Copyright 2025.

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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	trainingv1alpha1 "Shai1-Levi/githubissues-operator.git/api/v1alpha1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// GithubIssueReconciler reconciles a GithubIssue object
type GithubIssueReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=training.redhat.com,resources=githubissues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=training.redhat.com,resources=githubissues/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=training.redhat.com,resources=githubissues/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Reconcile function compare the state specified by
// the GithubIssue object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile

func (r *GithubIssueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Begin GithubIssue Reconcile")
	defer log.Info("Finish GithubIssue Reconcile")
	
	// Reconcile requeue results
	emptyResult := ctrl.Result{}

	// Step 1: Create Kubernetes config
	config, err := GetKubeConfig()
	if err != nil {
		fmt.Printf("Error getting Kubernetes config: %v", err)
	}

	// Step 2: Create Kubernetes clientset
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating Kubernetes clientset: %v", err)
	}

	accessToken, err := GetSecretData(clientSet, "my-secret", "githubissues-operator-system")

	// Fetch issues from GitHub
	body, err := r.fetchGitHubIssues(accessToken)
	if err != nil {
		log.Info("Failed to fetch GitHub issues")
		return ctrl.Result{}, nil
	}

	// Define a generic map for GitHubIssues
	var GitHubIssues []map[string]interface{}

	// Parse the JSON
	err = json.Unmarshal(body, &GitHubIssues)
	if err != nil {
		log.Info("err\n")
	}

	// Fetch the GithubIssue instance
	ghi := &trainingv1alpha1.GithubIssue{}
	if err := r.Get(ctx, req.NamespacedName, ghi); err != nil {
		if apiErrors.IsNotFound(err) {
			// FenceAgentsRemediation CR was not found, and it could have been deleted after reconcile request.
			// Return and don't requeue
			log.Info("GithubIssue CR was not found", "CR Name", req.Name, "CR Namespace", req.Namespace)
			return emptyResult, nil
		}
		log.Error(err, "Failed to get GithubIssue CR")
		return emptyResult, err
	}

	// // At the end of each Reconcile we try to update CR's status
	// defer func() {
	// 	if updateErr := r.updateStatus(ctx, ghi); updateErr != nil {
	// 		if apiErrors.IsConflict(updateErr) {
	// 			log.Info("Conflict has occurred on updating the CR status")
	// 		}
	// 		finalErr = utilErrors.NewAggregate([]error{updateErr, finalErr})
	// 	}
	// }()

	// name of our custom finalizer
	myFinalizerName := "github-issue.kubebuilder.io/finalizer"

	// examine DeletionTimestamp to determine if object is under deletion
	if ghi.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then let's add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(ghi, myFinalizerName) {
			controllerutil.AddFinalizer(ghi, myFinalizerName)
			log.Info("AddingFinalizer")
			if err := r.Update(ctx, ghi); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(ghi, myFinalizerName) {
			// our finalizer is present, so let's handle any external dependency
			title := ghi.Spec.Title
			description := ghi.Spec.Description

			var i int
			var url string
			url = ""
			// Print all keys and values dynamically
			for i = 0; i < len(GitHubIssues); i++ {
				item := GitHubIssues[i]
				fmt.Printf("\nIssue %d:\n", i)
				titleStr := fmt.Sprintf(item["title"].(string))
				isOpen := fmt.Sprintf(item["state"].(string))
				url = fmt.Sprintf(item["url"].(string))
				if (strings.TrimRight(string(titleStr), "\n") == title) && isOpen == "open" {
					if _, err := r.closeGithubIssue(title, description, url, accessToken); err != nil {
						// if fail to delete the external dependency here, return with error
						// so that it can be retried.
						return ctrl.Result{}, err
					}
					break
				}
			}

			log.Info("RemoveFinalizer")

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(ghi, myFinalizerName)
			if err := r.Update(ctx, ghi); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Extract `spec` field from cr
	title := ghi.Spec.Title
	description := ghi.Spec.Description
	repo := ghi.Spec.Repo

	var i int

	for i = 0; i < len(GitHubIssues); i++ {
		item := GitHubIssues[i]
		fmt.Printf("\nIssue %d:\n", i)
		titleStr := fmt.Sprintf(item["title"].(string))
		isOpen := fmt.Sprintf(item["state"].(string))
		if (strings.TrimRight(string(titleStr), "\n") == title) && isOpen == "open" {
			break
		}
	}

	// validate if the requiered GitHub issue is not exists when GitHub issues are empty or not
	if len(GitHubIssues) == 0 || i == len(GitHubIssues) {
		r.createGithubIssue(title, description, repo, accessToken)
		log.Info("Reconciling createGithubIssue")
	}

	return ctrl.Result{}, nil
}

// GetSecretData searches for the GitHub's secret, and then returns its decoded token.
func GetSecretData(clientSet *kubernetes.Clientset, secretName, secretNamespace string) (string, error) {
	// Get the secret from the specified namespace
	secret, err := clientSet.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Error getting secret: %v", err)
	}

	// Extract the token value
	tokenBase64, exists := secret.Data["token"] // "token" is the key inside the secret
	if !exists {
		fmt.Println("Token key not found in secret")
	}

	// Decode the token
	token := string(tokenBase64) // Secret data is already in []byte format
	return token, nil
}

// GetKubeConfig returns the appropriate Kubernetes config
func GetKubeConfig() (*rest.Config, error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		// Running inside a cluster
		return rest.InClusterConfig()
	}
	// Running outside cluster (local development)
	return clientcmd.BuildConfigFromFlags("", "/home/slevi/.kube/config")
}

func (r *GithubIssueReconciler) closeGithubIssue(title string, description string, url string, accessToken string) (ctrl.Result, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)

	// JSON payload for the issue
	jsonStr := fmt.Sprintf("{\"title\":\"%s\", \"body\":\"%s\", \"state\":\"closed\"}", title, description)

	// Create a new HTTP request
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer([]byte(jsonStr)))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Add("Authorization", "token "+tokenStr)
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	// The GitHub REST API is versioned.
	// The API version name is based on the date when the API version was released.
	// For example, the API version 2022-11-28 was released on Mon, 28 Nov 2022.
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	// Create HTTP client and send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("error reading token: \n")
		return ctrl.Result{}, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		return ctrl.Result{}, fmt.Errorf("GitHub API returned status: %d", resp.StatusCode)
	}

	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) createGithubIssue(title string, description string, repo string, accessToken string) (ctrl.Result, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)

	// JSON payload for the issue
	jsonStr := fmt.Sprintf("{\"title\":\"%s\", \"body\":\"%s\", \"state\":\"open\"}", title, description)
	// url := "https://api.github.com/repos/Shai1-Levi/githubissues-operator/issues"

	// Create a new HTTP request
	req, err := http.NewRequest("POST", repo, bytes.NewBuffer([]byte(jsonStr)))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Add("Authorization", "token "+tokenStr)
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	// The GitHub REST API is versioned.
	// The API version name is based on the date when the API version was released.
	// For example, the API version 2022-11-28 was released on Mon, 28 Nov 2022.
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

	// Create HTTP client and send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("error reading token: \n")
		return ctrl.Result{}, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		return ctrl.Result{}, fmt.Errorf("GitHub API returned status: %d", resp.StatusCode)
	}

	return ctrl.Result{}, nil
}

// fetchGitHubIssues reads the token, sends the request, and returns the response body
func (r *GithubIssueReconciler) fetchGitHubIssues(accessToken string) ([]byte, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)
	url := "https://api.github.com/repos/Shai1-Levi/githubissues-operator/issues"

	// Create a new HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Add("Authorization", "token "+tokenStr)
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	return body, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GithubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trainingv1alpha1.GithubIssue{}).
		Complete(r)
}
