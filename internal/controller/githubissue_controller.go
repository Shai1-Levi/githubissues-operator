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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
	"strings"

	// for listing CRD, go provides client which is different from
	// "kubernetes.clientset"
	// This clientset will be used to list down the existing CRD
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
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

	// 1. Create custom crdClientSet
	// here restConfig is your .kube/config file
	// Path to your kubeconfig file
	kubeconfig := "/home/slevi/.kube/config"

	// Load the kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Println("Error loading kubeconfig:", err)
		os.Exit(1)
	}

	crdClientSet, err := clientset.NewForConfig(config)
	if err != nil {
		return ctrl.Result{}, nil
	}

	// 2. List down all the existing crd in the cluster
	crdList, err := crdClientSet.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return ctrl.Result{}, nil
	}

	fmt.Print("Found")
	fmt.Print(len(crdList.Items))

	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Println("Error creating dynamic client:", err)
		os.Exit(1)
	}

	// Define the GroupVersionResource (GVR) for your CRD
	gvr := schema.GroupVersionResource{
		Group:    "training.redhat.com",
		Version:  "v1alpha1",
		Resource: "githubissues", // Plural form of your CRD
	}

	// Fetch a specific CR instance (replace "githubissue-sample" with your CR name)
	crName := "example-issue"
	cr, err := dynClient.Resource(gvr).Namespace("default").Get(context.TODO(), crName, metav1.GetOptions{})
	if err != nil {
		fmt.Println("Error fetching Custom Resource:", err)
		os.Exit(1)
	}

	// Extract `spec` field
	title, found, err := unstructured.NestedString(cr.Object, "spec", "title")
	description, found, err := unstructured.NestedString(cr.Object, "spec", "description")
	if err != nil || !found {
		fmt.Println("Error retrieving spec:", err)
		os.Exit(1)
	}

	// Print the spec content
	fmt.Println("Spec of", crName, ":", title)

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
		titleStr := fmt.Sprintf(item["title"].(string))
		isOpen := fmt.Sprintf(item["state"].(string))
		// url := fmt.Sprintf(item["url"].(string))
		if (strings.TrimRight(string(titleStr), "\n") == title) && isOpen == "open" {
			break
		}
	}

	if false{
		r.closeGithubIssue(title, description, url)
		log.Info("Reconciling closeGithubIssue")
	}

	// validate if the requiered GitHub issue is not exists when GitHub issues are empty or not
	if len(result) == 0 || i == len(result) {
		r.createGithubIssue(title, description)
		log.Info("Reconciling createGithubIssue")
	}

	return ctrl.Result{}, nil
}

func (r *GithubIssueReconciler) closeGithubIssue(title string, description string, url string) (ctrl.Result, error) {
	// Read the token from file

	tokenBytes, err := os.ReadFile("github_token")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reading token: %w", err)
	}

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(string(tokenBytes))

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

func (r *GithubIssueReconciler) createGithubIssue(title string, description string) (ctrl.Result, error) {
	// Read the token from file

	tokenBytes, err := os.ReadFile("github_token")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error reading token: %w", err)
	}

	// Trim spaces and newlines from the token
	tokenStr := strings.TrimSpace(string(tokenBytes))

	// JSON payload for the issue
	jsonStr := fmt.Sprintf("{\"title\":\"%s\", \"body\":\"%s\", \"state\":\"open\"}", title, description)
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
