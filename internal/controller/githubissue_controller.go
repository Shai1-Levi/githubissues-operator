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
	log.Info("Reconciling GithubIssue")

	// Fetch issues from GitHub
	body, err := r.fetchGitHubIssues()
	if err != nil {
		log.Info("Failed to fetch GitHub issues")
		return ctrl.Result{}, nil
	}

	// Define a generic map
	var result []map[string]interface{}

	// Parse the JSON
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Info("err\n")
	}

	var i int

	// Print all keys and values dynamically
	for i = 0; i < len(result); i++ {
		item := result[i]
		fmt.Printf("\nIssue %d:\n", i)
		for key, value := range item {

			// check github issue is exist
			if ValueStr, ok := value.(string); ok {
				if strings.TrimRight(string(ValueStr), "\n") == "Buy cheese and bread for breakfast." {
					fmt.Printf("  %s: %v\n", key, ValueStr)
				}
			} else {
				// github issue is not exist, create new issue
				continue
			}
		}
	}

	// validate if the GitHub issue if the requiered issue is not exists when GitHub issues are empty or not
	if len(result) == 0 || i == len(result) {
		r.createGithubIssue()
		log.Info("Reconciling createGithubIssue")
	}

	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) createGithubIssue() (ctrl.Result, error) {
	// Read the token from file

	tokenBytes, err := os.ReadFile("github_token")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reading token: %w", err)
	}

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(string(tokenBytes))

	// JSON payload for the issue
	jsonStr := `{"title":"Buy cheese and bread for breakfast2.", "body":"test2"}`
	url := "https://api.github.com/repos/Shai1-Levi/githubissues-operator/issues"

	// Create a new HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(jsonStr)))
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

	// Read and log the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reading response: %w", err)
	}

	fmt.Println("GitHub API Response:", string(body))

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		return ctrl.Result{}, fmt.Errorf("GitHub API returned status: %d", resp.StatusCode)
	}

	return ctrl.Result{}, nil
}

// fetchGitHubIssues reads the token, sends the request, and returns the response body
func (r *GithubIssueReconciler) fetchGitHubIssues() ([]byte, error) {
	// Read the token from file
	tokenBytes, err := os.ReadFile("github_token")
	if err != nil {
		return nil, fmt.Errorf("error reading token: %w", err)
	}

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(string(tokenBytes))
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
