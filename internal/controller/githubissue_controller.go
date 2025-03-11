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
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GithubIssue object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *GithubIssueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling GithubIssue SHAI", "name", req.NamespacedName)

	// // TODO(user): your logic here

	// // Read the token from the file
	// tokenBytes, err := os.ReadFile("github_token")
	// if err != nil {
	// 	fmt.Println("Error reading token:", err)
	// 	return ctrl.Result{}, nil
	// }

	// // Trim spaces and newlines from the token
	// tokenStr := strings.TrimSpace(string(tokenBytes))

	// url := "https://api.github.com/repos/Shai1-Levi/githubissues-operator/issues"

	// // Create a new HTTP request
	// req2, err := http.NewRequest("GET", url, nil)

	// // Set headers
	// req2.Header.Add("Authorization", "token "+tokenStr)
	// req2.Header.Add("Accept", "application/vnd.github.v3+json")

	// log.Info("log SHAI", "name", req2)

	// //send request and get the response
	// client := &http.Client{}
	// resp, err := client.Do(req2)
	// if err != nil {
	// 	fmt.Println("Error:", err)
	// 	return ctrl.Result{}, nil
	// }
	// defer resp.Body.Close()

	// // Read response body
	// body, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	fmt.Println("Error reading response:", err)
	// 	return ctrl.Result{}, nil
	// }

	// Fetch issues from GitHub
	body, err := r.fetchGitHubIssues()
	if err != nil {
		log.Info("Failed to fetch GitHub issues")
		return ctrl.Result{}, nil
	}

	log.Info("Answerrrr SHAI")
	// log.Info(string(body))

	// Define a generic map
	var result []map[string]interface{}

	// Parse the JSON
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Info("err\n")
	}

	// Print all keys and values dynamically
	for i, item := range result {
		fmt.Printf("\nIssue %d:\n", i+1)
		for key, value := range item {

			// check github issue is exist
			if ValueStr, ok := value.(string); ok {
				if strings.TrimRight(string(ValueStr), "\n") == "Generate scaffold files by operator-sdk" {
					fmt.Printf("  %s: %v\n", key, ValueStr)
				}
			} else {
				// github issue is not exist, create new issue
				r.createGithubIssue("sdfadsf")
				continue
			}
		}
	}

	return ctrl.Result{}, nil
}

// func (r *GithubIssueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
// 	log := log.FromContext(ctx)
// 	log.Info("Reconciling GithubIssue SHAI", "name", req.NamespacedName)

// 	// Fetch issues from GitHub
// 	body, err := r.fetchGitHubIssues()
// 	if err != nil {
// 		log.Info(err, "Failed to fetch GitHub issues")
// 		return ctrl.Result{}, nil
// 	}

// 	// Process the issues
// 	r.processGitHubIssues(body)

// 	return ctrl.Result{}, nil
// }

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

// // processGitHubIssues parses the JSON response and checks for a specific issue
// func (r *GithubIssueReconciler) processGitHubIssues(body []byte) {
// 	var issues []map[string]interface{}

// 	// Parse the JSON response
// 	err := json.Unmarshal(body, &issues)
// 	if err != nil {
// 		log.Info(err, "Failed to parse GitHub issues JSON")
// 		return
// 	}

// 	// Iterate over the issues
// 	for i, issue := range issues {
// 		log.Info(fmt.Sprintf("\nIssue %d:", i+1))

// 		for key, value := range issue {
// 			// Check if the GitHub issue exists
// 			if valueStr, ok := value.(string); ok {
// 				if strings.TrimSpace(valueStr) == "Generate scaffold files by operator-sdk" {
// 					log.Info(fmt.Sprintf("  %s: %v", key, valueStr))
// 				}
// 			} else {
// 				// GitHub issue does not exist, create a new issue (TODO: Implement this)
// 				continue
// 			}
// 		}
// 	}
// }

// SetupWithManager sets up the controller with the Manager.
func (r *GithubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trainingv1alpha1.GithubIssue{}).
		Complete(r)
}
