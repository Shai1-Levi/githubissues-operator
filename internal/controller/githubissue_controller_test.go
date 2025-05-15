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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	trainingv1alpha1 "Shai1-Levi/githubissues-operator.git/api/v1alpha1"
	// ghiConroller "Shai1-Levi/githubissues-operator.git/internal/controller"
)

var _ = Describe("GithubIssue Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		githubissue := &trainingv1alpha1.GithubIssue{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind GithubIssue")
			err := k8sClient.Get(ctx, typeNamespacedName, githubissue)
			if err != nil && errors.IsNotFound(err) {
				// resource := &trainingv1alpha1.GithubIssue{
				// 	ObjectMeta: metav1.ObjectMeta{
				// 		Name:      resourceName,
				// 		Namespace: "default",
				// 	},
				// 	// TODO(user): Specify other spec details if needed.
				// }
				resource := getTestCR(resourceName, "https://github.com/Shai1-Levi/githubissues-operator", 
				"Test issue from test framework", "Test body from test framework")
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &trainingv1alpha1.GithubIssue{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance GithubIssue")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &GithubIssueReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("Functionality test", func() {
		const resourceName = "test-resource"
		accessToken := os.Getenv("SECRET_Token") // Read the environment variable
		var cr *trainingv1alpha1.GithubIssue
		var controllerReconciler *GithubIssueReconciler

		Context("Testing create", func() {
			BeforeEach(func() {
	

				controllerReconciler = &GithubIssueReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				cr = getTestCR(resourceName, "https://github.com/Shai1-Levi/githubissues-operator", 
				"Test issue from test framework", "Test body from test framework")
			})
			When("CR was initalized", func() {
				It("should be create a GitHub issue if not exists", func() {
					annotationValue, err := controllerReconciler.createGithubIssue(cr.Spec.Title, cr.Spec.Description, cr.Spec.Repo, accessToken)
					Expect(annotationValue).NotTo(BeZero())
					Expect(err).NotTo(HaveOccurred())
				})
			})

			When("CR was updated", func() {
				It("should be update the GitHub issue if exists", func() {
					needUpdate, err := controllerReconciler.updateGitHubIssue(cr.Spec.Title, cr.Spec.Description, cr.Spec.Repo, "-50", accessToken)
					Expect(needUpdate).To(BeFalse())
					Expect(err).To(HaveOccurred())
				})
			})

			When("CR was deleted", func() {
				It("should be delete the GitHub issue if exists", func() {
					err := controllerReconciler.closeGithubIssueFromCR(cr, accessToken)
					Expect(err).To(HaveOccurred())
				})
			})

			When("CR failed to create the GitHub issue", func() {
				It("should not create a GitHub issue and return error", func(){
					annotationValue, err := controllerReconciler.createGithubIssue(cr.Spec.Title, cr.Spec.Description, "sdfgdfgfdgdfgfdgfdg", accessToken)
					Expect(annotationValue).To(Equal(""))
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})

})

func getTestCR(crName, repo, title, description string) *trainingv1alpha1.GithubIssue {
	return &trainingv1alpha1.GithubIssue{
		ObjectMeta: metav1.ObjectMeta{
			Name: crName,
		},
		Spec: trainingv1alpha1.GithubIssueSpec{
			Repo: repo,
			Title: title,
			Description: description,
		},
	}
}

