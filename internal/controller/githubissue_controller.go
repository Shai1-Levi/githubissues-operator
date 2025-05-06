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
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	trainingv1alpha1 "Shai1-Levi/githubissues-operator.git/api/v1alpha1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
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

// Define a struct to hold the relevant parts of the GitHub Search API response.
// We only care about total_count for this example.
type GitHubSearchResponse struct {
	TotalCount int                      `json:"total_count"` // Maps the JSON key "total_count" to this field
	Items      []map[string]interface{} `json:"items"`
}

// Define a struct to hold the relevant parts of the GitHub Search API response.
// We only care about url for this example.
type GitHubCreateResponse struct {
	Url string `json:"url"` // Maps the JSON key "url" to this field
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

	accessToken := os.Getenv("SECRET_Token") // Read the environment variable

	if accessToken == "" {
		fmt.Println("SECRET_Token is not set")
		return ctrl.Result{}, nil
	}

	// Fetch issues from GitHub
	body, err := r.fetchOpenGitHubIssues(accessToken)
	if err != nil {
		log.Info("Failed to fetch GitHub issues")
		return ctrl.Result{}, nil
	}

	// 1. Create an instance of our struct
	var GitHubIssues GitHubSearchResponse

	// 2. Unmarshal the JSON data into the struct
	err = json.Unmarshal(body, &GitHubIssues)
	if err != nil {
		fmt.Printf("Error unmarshaling JSON: %v\n", err)
		// You might want to print bodyBytes here to see what was received if it's not valid JSON
		// fmt.Printf("Raw response: %s\n", string(bodyBytes))
		return emptyResult, err
	}

	// 3. Access the TotalCount field
	fmt.Printf("Successfully unmarshaled JSON.\n")
	fmt.Printf("Total Count of Issues: %d\n", GitHubIssues.TotalCount)

	// Get count in the reconciled resource's namespace
	countInNamespace, err := r.getGithubIssueCountCR(ctx, req.Namespace)
	if err != nil {
		// Handle error appropriately (e.g., requeue)
		log.Error(err, "Failed to get GithubIssue count in namespace", "namespace", req.Namespace)
		// Depending on your logic, you might want to requeue or continue
		return ctrl.Result{}, err // Example: Requeue on error
	}

	fmt.Printf("countInNamespace")
	fmt.Print(countInNamespace)

	if countInNamespace < GitHubIssues.TotalCount {
		log.Info("There are missing Isuues, represent them on CRs")
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
			for i = 0; i < len(GitHubIssues.Items); i++ {
				item := GitHubIssues.Items[i]
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
		return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
	}

	// Extract `spec` field from cr
	title := ghi.Spec.Title
	description := ghi.Spec.Description
	repo := ghi.Spec.Repo

	var i int

	for i = 0; i < len(GitHubIssues.Items); i++ {
		item := GitHubIssues.Items[i]
		fmt.Printf("\nIssue %d:\n", i)
		titleStr := fmt.Sprintf(item["title"].(string))
		isOpen := fmt.Sprintf(item["state"].(string))
		if (strings.TrimRight(string(titleStr), "\n") == title) && isOpen == "open" {
			break
		}
	}

	// validate if the requiered GitHub issue is not exists when GitHub issues are empty or not
	if GitHubIssues.TotalCount == 0 || i == GitHubIssues.TotalCount {
		annotationValue, err := r.createGithubIssue(title, description, repo, accessToken)
		if annotationValue == "" {
			fmt.Printf("annotation value is empty string something went wrong")
			return ctrl.Result{}, err
		}
		annotationKey := "github-issue.kubebuilder.io/issue-number"
		// annotationValue := "5555" // Current timestamp

		if err := r.UpdateGithubIssueAnnotation(ctx, req, annotationKey, annotationValue); err != nil {
			// Handle error, potentially requeue
			return ctrl.Result{}, err
		}
		log.Info("Reconciling createGithubIssue")
	}

	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
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

// Function to add or update an annotation on a GithubIssue CR
func (r *GithubIssueReconciler) UpdateGithubIssueAnnotation(
	ctx context.Context,
	req ctrl.Request,
	annotationKey string,
	annotationValue string,
) error {
	logger := log.FromContext(ctx).WithValues("githubissue", req.NamespacedName)

	// 1. Get the GithubIssue CR
	githubIssue := &trainingv1alpha1.GithubIssue{}
	if err := r.Get(ctx, req.NamespacedName, githubIssue); err != nil {
		if apiErrors.IsNotFound(err) {
			logger.Info("GithubIssue not found, cannot update annotation.")
			return nil // Or return error if this is unexpected
		}
		logger.Error(err, "Failed to get GithubIssue to update annotation")
		return err
	}

	// 2. Modify Annotations
	// Initialize the Annotations map if it's nil
	if githubIssue.ObjectMeta.Annotations == nil {
		githubIssue.ObjectMeta.Annotations = make(map[string]string)
	}

	// Check if the annotation value actually needs updating
	currentValue, exists := githubIssue.ObjectMeta.Annotations[annotationKey]
	if exists && currentValue == annotationValue {
		logger.Info("Annotation already has the desired value, no update needed.", "key", annotationKey, "value", annotationValue)
		return nil
	}

	// Add or update the annotation
	githubIssue.ObjectMeta.Annotations[annotationKey] = annotationValue
	logger.Info("Updating annotation", "key", annotationKey, "value", annotationValue)

	// 3. Update the GithubIssue CR
	if err := r.Client.Update(ctx, githubIssue); err != nil {
		logger.Error(err, "Failed to update GithubIssue with new annotation")
		return err
	}

	logger.Info("Successfully updated GithubIssue annotation", "key", annotationKey, "value", annotationValue)
	return nil
}

func (r *GithubIssueReconciler) createGithubIssue(title string, description string, repo string, accessToken string) (string, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)

	// JSON payload for the issue
	jsonStr := fmt.Sprintf("{\"title\":\"%s\", \"body\":\"%s\", \"state\":\"open\"}", title, description)

	// Create a new HTTP request
	req, err := http.NewRequest("POST", repo, bytes.NewBuffer([]byte(jsonStr)))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
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
		return "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("GitHub API returned status: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	// Declare a map to hold the unmarshaled JSON
	var result map[string]interface{}

	// Unmarshal the JSON data into the map
	err = json.Unmarshal(body, &result)
	if err != nil {
		fmt.Printf("Error unmarshaling JSON: %v", err)
	}

	// Access the "url" field
	// You need to perform a type assertion to get the string value
	urlValue, ok := result["url"]
	if !ok {
		fmt.Printf("'url' field not found in JSON response")
	}

	urlString, ok := urlValue.(string)
	if !ok {
		fmt.Printf("'url' field is not a string, actual type: %T", urlValue)
	}

	fmt.Println("Extracted URL (from map):", urlString)

	_, numberStr, err := r.extractIssueNumberFromString(urlString)
	if err != nil {
		fmt.Printf("Error unmarshaling JSON: %v\n", err)
		// You might want to print bodyBytes here to see what was received if it's not valid JSON
		// fmt.Printf("Raw response: %s\n", string(bodyBytes))
		return "", err
	}

	return numberStr, nil
}

func (r *GithubIssueReconciler) extractIssueNumberFromString(s string) (int, string, error) {
	lastSlashIndex := strings.LastIndex(s, "/")
	if lastSlashIndex == -1 {
		return 0, "", fmt.Errorf("no '/' found in string")
	}

	// Extract the part after the last slash
	potentialNumberStr := s[lastSlashIndex+1:]
	if potentialNumberStr == "" {
		return 0, "", fmt.Errorf("string ends with '/', no number found after it")
	}

	// Attempt to convert the extracted part to an integer
	number, err := strconv.Atoi(potentialNumberStr)
	if err != nil {
		return 0, potentialNumberStr, fmt.Errorf("could not convert '%s' to an integer: %w", potentialNumberStr, err)
	}

	return number, potentialNumberStr, nil
}

// Function to get the count of GithubIssue CRs
func (r *GithubIssueReconciler) getGithubIssueCountCR(ctx context.Context, namespace string) (int, error) {
	logger := log.FromContext(ctx)

	// 1. Create an empty list object for GithubIssue
	githubIssueList := &trainingv1alpha1.GithubIssueList{} // Use the specific List type

	// 2. Define List Options (namespace, label selectors, etc.)
	listOpts := []client.ListOption{}
	if namespace != "" {
		// List only in the specified namespace
		listOpts = append(listOpts, client.InNamespace(namespace))
		logger.Info("Listing GithubIssues in namespace", "namespace", namespace)
	} else {
		// List across all namespaces (requires cluster-level list permissions)
		logger.Info("Listing GithubIssues across all namespaces")
		// No namespace option needed, or explicitly use client.InNamespace("")
	}

	// Optionally add label selectors:
	// listOpts = append(listOpts, client.MatchingLabels{"repo": "owner/repo-name"})

	// 3. Call the List method using the controller's client
	err := r.Client.List(ctx, githubIssueList, listOpts...)
	if err != nil {
		logger.Error(err, "Failed to list GithubIssues")
		return 0, err // Return error
	}

	// 4. Get the count from the length of the Items slice
	count := len(githubIssueList.Items)
	logger.Info("Found GithubIssues", "count", count)

	return count, nil
}

// fetchOpenGitHubIssues reads the token, sends the request, and returns the response body
func (r *GithubIssueReconciler) fetchOpenGitHubIssues(accessToken string) ([]byte, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)
	url := "https://api.github.com/search/issues?q=repo:Shai1-Levi/githubissues-operator+type:issue+state:open"

	// Create a new HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Add("Authorization", "token "+tokenStr)
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")

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

	// // Store the output in a string variable
	// responseBodyString := string(body)

	// // Now you can use responseBodyString
	// fmt.Println("Response Body Fetch:")
	// fmt.Println(responseBodyString)

	return body, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GithubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trainingv1alpha1.GithubIssue{}).
		Complete(r)
}
