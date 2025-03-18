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

package v1alpha1

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var githubissuelog = logf.Log.WithName("githubissue-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (ghi *GithubIssue) SetupWebhookWithManager(mgr ctrl.Manager) error {
	ctrl.NewWebhookManagedBy(mgr).For(ghi).Complete()
	githubissuelog.Info("setup")
	return ctrl.NewWebhookManagedBy(mgr).
		For(ghi).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-training-redhat-com-v1alpha1-githubissue,mutating=false,failurePolicy=fail,sideEffects=None,groups=training.redhat.com,resources=githubissues,verbs=create;update,versions=v1alpha1,name=vgithubissue.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &GithubIssue{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (ghi *GithubIssue) ValidateCreate() (admission.Warnings, error) {
	githubissuelog.Info("validate create", "name")

	// TODO(user): fill in your validation logic upon object creation.
	return validateGHI(&ghi.Spec)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (ghi *GithubIssue) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	githubissuelog.Info("validate update", "name")

	// TODO(user): fill in your validation logic upon object update.
	return validateGHI(&ghi.Spec)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (ghi *GithubIssue) ValidateDelete() (admission.Warnings, error) {
	githubissuelog.Info("validate delete", "name")

	return nil, nil
}

func validateGHI(ghiSpec *GithubIssueSpec) (admission.Warnings, error) {
	aggregated := errors.NewAggregate([]error{validateRepoUrl(ghiSpec.Repo)})

	return admission.Warnings{}, aggregated
}

func validateRepoUrl(repo string) error {

	url := "https://api.github.com/repos/Shai1-Levi/githubissues-operator/issues"

	// Create a new HTTP request
	exists, err := http.NewRequest("GET", url, nil)
	fmt.Println("##################################################")
	fmt.Println(exists)
	fmt.Println(err)
	if err != nil {
		return errors.NewAggregate([]error{
			fmt.Errorf("Failed to validate fence agent: %s. You might want to try again.", repo),
			err,
		})
	}
	// if !exists {
	// 	return fmt.Errorf("unsupported fence agent: %s", repo)
	// }
	return nil
}
