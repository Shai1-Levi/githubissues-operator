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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	trainingv1alpha1 "Shai1-Levi/githubissues-operator.git/api/v1alpha1"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	annotationKey   = "github-issue.kubebuilder.io/issue-number"
	myFinalizerName = "github-issue.kubebuilder.io/finalizer"
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

	// Fetch the GithubIssue instance
	ghi := &trainingv1alpha1.GithubIssue{}
	if err := r.Get(ctx, req.NamespacedName, ghi); err != nil {
		if apiErrors.IsNotFound(err) {
			// GitHubIssue CR was not found, and it could have been deleted after reconcile request.
			// Return and don't requeue
			logStr := fmt.Sprintf("GithubIssue CR was not found, CR Name %s CR Namespace %s", req.Name, req.Namespace)
			log.Info(logStr)
			return emptyResult, nil
		}
		log.Error(err, "Failed to get GithubIssue CR")
		return emptyResult, err
	}

	if !ghi.ObjectMeta.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(ghi, myFinalizerName) {
		// When CR doesn't include a finalizer and the CR deletionTimestamp exists
		// then we can skip update, since it will be removed soon.
		return emptyResult, nil
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if ghi.ObjectMeta.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(ghi, myFinalizerName) {
		// The object is not being deleted, so if it does not have our finalizer,
		// then let's add the finalizer and update the object. This is equivalent
		// to registering our finalizer.

		controllerutil.AddFinalizer(ghi, myFinalizerName)
		log.Info("AddingFinalizer")
		if err := r.Update(ctx, ghi); err != nil {
			return emptyResult, err
		}
		return emptyResult, nil
	}

	accessToken := os.Getenv("SECRET_Token") // Read the environment variable

	if accessToken == "" {
		log.Info("SECRET_Token is not set")
		return emptyResult, nil
	}

	// Extract `spec` field from cr
	title := ghi.Spec.Title
	description := ghi.Spec.Description
	repo := string(ghi.Spec.Repo) + "/issues"

	fmt.Printf("Title %s Description %s Repo %s \n", title, description, repo)

	// Fetch issues from GitHub
	body, err := r.fetchGitHubIssues(ghi.Spec.Repo, accessToken)
	if err != nil {
		log.Info("Failed to fetch GitHub issues")
		return emptyResult, nil
	}

	var gitHubIssues GitHubSearchResponse

	// Parse the JSON
	err = json.Unmarshal(body, &gitHubIssues)
	if err != nil {
		fmt.Print(err)
		log.Info("Failed to parase response to JSon")
	}

	// The object is being deleted
	if !ghi.ObjectMeta.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(ghi, myFinalizerName) {
		// Delete CR only when a finalizer and DeletionTimestamp are set
		// our finalizer is present, handle any external dependency

		if err := r.closeGithubIssueFromCR(ghi, accessToken); err != nil {
			// if fail to delete the external dependency here, return with error
			// so that it can be retried.
			return emptyResult, err
		}

		log.Info("Trying RemoveFinalizer")

		// remove our finalizer from the list and update it.
		controllerutil.RemoveFinalizer(ghi, myFinalizerName)
		if err := r.Update(ctx, ghi); err != nil {
			return emptyResult, err
		}
		log.Info("RemoveFinalizer")
		// Stop reconciliation as the item is being deleted
		return emptyResult, nil
	}

	if r.hasSpecificAnnotation(ghi) {
		fmt.Printf("CR has the annotation key %s \n", annotationKey)

		// 3. (Optional) Get the value of the annotation
		value, _ := r.getSpecificAnnotationValue(ghi)
		fmt.Printf("Annotation value key %s value %s \n", annotationKey, value)

		// Now you can act based on the presence or value of the annotation
		if value != "" {
			// Do something specific
			log.Info("Annotation value is true, performing action...")
			ghiBySerial, err := r.fetchGitHubIssuesbyIssueNumber(value, repo, accessToken)
			if err != nil {
				fmt.Print(err)
				log.Info("Failed to parase response to JSon")
			}

			// Declare a map to hold the unmarshaled JSON
			var result map[string]interface{}

			// Unmarshal the JSON data into the map
			err = json.Unmarshal(ghiBySerial, &result)
			if err != nil {
				fmt.Printf("Error unmarshaling JSON: %v", err)
			}
			if result["title"] != title || result["body"] != description {
				needUpdate, err := r.updateGitHubIssue(title, description, repo, value, accessToken)
				if err != nil {
					return emptyResult, err
				}
				if !needUpdate {
					return emptyResult, nil
				}
			}

			return emptyResult, nil

		}

	} else { // No anttotaion filed, hence CR is on creation step
		log.Info("CR does not have the annotation", "key", annotationKey)

		annotationValue, err := r.createGithubIssue(title, description, repo, accessToken)
		if annotationValue == "" {
			fmt.Printf("annotation value is empty string something went wrong")
			return emptyResult, err
		}

		if err := r.UpdateGithubIssueAnnotation(ctx, req, annotationValue); err != nil {
			return emptyResult, err
		}
		log.Info("Reconciling createGithubIssue")

		return emptyResult, nil
	}

	return emptyResult, nil

}

func (r *GithubIssueReconciler) updateGitHubIssue(title string, description string, repo string, issueNumber string, accessToken string) (bool, error) {

	fmt.Print(repo)

	// url := fmt.Sprintf("https://api.github.com/repos/Shai1-Levi/githubissues-operator/issues/%s", issueNumber)
	url := repo + "/" + issueNumber

	ans, e := r.updateGitHubIssuefileds(title, description, url, accessToken)
	if e != nil {
		return false, fmt.Errorf("failed to update issue fields: %w", e)
	}

	return ans, nil

}

func (r *GithubIssueReconciler) hasSpecificAnnotation(obj metav1.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false // No annotations at all
	}
	_, exists := annotations[annotationKey]
	return exists
}

func (r *GithubIssueReconciler) getSpecificAnnotationValue(obj metav1.Object) (string, bool) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return "", false // No annotations at all
	}
	value, exists := annotations[annotationKey]
	return value, exists
}

// Function to add or update an annotation on a GithubIssue CR
func (r *GithubIssueReconciler) UpdateGithubIssueAnnotation(
	ctx context.Context,
	req ctrl.Request,
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

func (r *GithubIssueReconciler) closeGithubIssueFromCR(ghi *trainingv1alpha1.GithubIssue, accessToken string) error {

	title := ghi.Spec.Title
	description := ghi.Spec.Description
	repo := ghi.Spec.Repo + "/issues"

	// 3. (Optional) Get the value of the annotation
	annotationValue, _ := r.getSpecificAnnotationValue(ghi)
	fmt.Printf("Annotation value key %s value %s \n", annotationKey, annotationValue)

	// Now you can act based on the presence or value of the annotation
	if annotationValue != "" {
		// Do something specific
		fmt.Printf("Annotation value is true, performing action...")
		if _, err := r.closeGithubIssue(title, description, repo, annotationValue, accessToken); err != nil {
			// if fail to delete the external dependency here, return with error
			// so that it can be retried.
			return err
		}
	}

	return nil
}

func (r *GithubIssueReconciler) updateGitHubIssuefileds(title string, description string, repo string, accessToken string) (bool, error) {
	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)

	// JSON payload for the issue
	type IssuePayload struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		State string `json:"state"`
	}

	payload := IssuePayload{
		Title: title,
		Body:  description,
		State: "open",
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("error marshaling JSON: %w", err)
	}
	jsonStr := string(jsonData)

	// Create a new HTTP request
	req, err := http.NewRequest("PATCH", repo, bytes.NewBuffer([]byte(jsonStr)))
	if err != nil {
		return false, fmt.Errorf("error creating request: %w", err)
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
		return false, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("GitHub API returned status: %d", resp.StatusCode)
	}

	return true, nil
}

func (r *GithubIssueReconciler) closeGithubIssue(title string, description string, url string, issueNumber string, accessToken string) (ctrl.Result, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)

	// JSON payload for the issue
	type IssuePayload struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		State string `json:"state"`
	}

	payload := IssuePayload{
		Title: title,
		Body:  description,
		State: "closed",
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error marshaling JSON: %w", err)
	}
	jsonStr := string(jsonData)

	repo := url + "/" + issueNumber

	// Create a new HTTP request
	req, err := http.NewRequest("PATCH", repo, bytes.NewBuffer([]byte(jsonStr)))
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("error closing response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return ctrl.Result{}, fmt.Errorf("GitHub API returned status: %d", resp.StatusCode)
	}

	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) createGithubIssue(title string, description string, repo string, accessToken string) (string, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)

	// JSON payload for the issue
	type IssuePayload struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		State string `json:"state"`
	}

	payload := IssuePayload{
		Title: title,
		Body:  description,
		State: "open",
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error marshaling JSON: %w", err)
	}
	jsonStr := string(jsonData)

	// url := repo + "/issues"

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

	_, numberStr, err := r.extractIssueNumberFromString(urlString)
	if err != nil {
		fmt.Printf("Error unmarshaling JSON: %v\n", err)
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

func (r *GithubIssueReconciler) getStringAfterRepos(url string) (string, bool) {
	searchText := "repos/"
	index := strings.Index(url, searchText)

	if index == -1 {
		// "repos/" not found in the string
		return "", false
	}

	// Calculate the starting position of the substring after "repos/"
	startIndex := index + len(searchText)

	// Ensure startIndex is within bounds (though with "repos/" it's unlikely to be an issue if found)
	if startIndex >= len(url) {
		// "repos/" is at the very end, so nothing comes after it
		return "", true // Found "repos/", but nothing after
	}

	return url[startIndex:], true
}

// fetchGitHubIssues reads the token, sends the request, and returns the response body
func (r *GithubIssueReconciler) fetchGitHubIssues(repo, accessToken string) ([]byte, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)
	owner_repo, boolean := r.getStringAfterRepos(repo)
	if !boolean || owner_repo == "" {
		return nil, fmt.Errorf("failed to get owner_repo")
	}
	// url := "https://api.github.com/search/issues?q=repo:Shai1-Levi/githubissues-operator+type:issue+state:open"
	url := "https://api.github.com/search/issues?q=repo:" + owner_repo + "+type:issue+state:open"

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

	return body, nil
}

// fetchGitHubIssues reads the token, sends the request, and returns the response body
func (r *GithubIssueReconciler) fetchGitHubIssuesbyIssueNumber(issueNumber, repo, accessToken string) ([]byte, error) {

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(accessToken)
	url := repo + "/" + issueNumber

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

	return body, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GithubIssueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trainingv1alpha1.GithubIssue{}).
		Complete(r)
}
