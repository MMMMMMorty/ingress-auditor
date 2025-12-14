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

	"github.com/MMMMMMorty/ingress-auditor/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("IngressTLSLog Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceNameFailure = "test-resource-failure"

		ctx := context.Background()

		typeNamespacedNameFailure := types.NamespacedName{
			Name:      resourceNameFailure,
			Namespace: "default",
		}
		testIngressFailure := &networkingv1.Ingress{}

		const resourceNameSuccess = "test-resource-success"

		typeNamespacedNameSuccess := types.NamespacedName{
			Name:      resourceNameSuccess,
			Namespace: "default",
		}
		testIngressSuccess := &networkingv1.Ingress{}

		BeforeEach(func() {
			By("creating the ingress resource for failure test")
			err := k8sClient.Get(ctx, typeNamespacedNameFailure, testIngressFailure)
			if err != nil && errors.IsNotFound(err) {
				resource := &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFailure,
						Namespace: "default",
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "example.com",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Path: "/",
												PathType: func() *networkingv1.PathType {
													pt := networkingv1.PathTypePrefix
													return &pt
												}(),
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "nginx",
														Port: networkingv1.ServiceBackendPort{
															Number: 80,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			err = k8sClient.Get(ctx, typeNamespacedNameFailure, testIngressFailure)
			Expect(err).NotTo(HaveOccurred())

			By("creating the ingress resource for successful test")
			err = k8sClient.Get(ctx, typeNamespacedNameSuccess, testIngressSuccess)
			if err != nil && errors.IsNotFound(err) {
				resource := &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameSuccess,
						Namespace: "default",
						Annotations: map[string]string{
							"nginx.ingress.kubernetes.io/permanent-redirect": "https://success.test.example.com",
						},
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "success.example.com",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Path: "/",
												PathType: func() *networkingv1.PathType {
													pt := networkingv1.PathTypePrefix
													return &pt
												}(),
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "nginx-1",
														Port: networkingv1.ServiceBackendPort{
															Number: 80,
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			err = k8sClient.Get(ctx, typeNamespacedNameSuccess, testIngressSuccess)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &networkingv1.Ingress{}
			err := k8sClient.Get(ctx, typeNamespacedNameFailure, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ingress")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should be failed to reconcile the resource", func() {
			By("Reconciling the ingress with creating TLS log failure")
			controllerReconciler := &IngressTLSLogReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Interval:             3600,
				IngressErrorMap:      store.NewIngressErrorMap(),
				IngressUpdateTimeMap: store.NewIngressUpdateTimeMap(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameFailure,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create new TLS log"))

			// namespace where the project is deployed in
			const namespace = "ingress-auditor-system"

			By("Creating ingress-auditor-system namespapce")
			// Create a Namespace object
			namespaceResource := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			Expect(k8sClient.Create(ctx, namespaceResource)).To(Succeed())

			By("Reconciling the ingress with HTTP but no redirect")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameFailure,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TLS is not used and redirect is not applied neither"))

			By("Patching the ingress with empty TLS resource")
			patch := client.MergeFrom(testIngressFailure.DeepCopy())

			tlsEntry := networkingv1.IngressTLS{
				Hosts: []string{"example.com"},
			}

			testIngressFailure.Spec.TLS = append(testIngressFailure.Spec.TLS, tlsEntry)

			err = k8sClient.Patch(ctx, testIngressFailure, patch)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the ingress with the missing secretName field resource")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameFailure,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("the secretName does not define in ingress"))

			By("Patching the ingress with the secret resource")
			patch = client.MergeFrom(testIngressFailure.DeepCopy())

			testIngressFailure.Spec.TLS[0].SecretName = "test-secret"

			err = k8sClient.Patch(ctx, testIngressFailure, patch)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the ingress with the missing secret")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameFailure,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to fetch secret"))

			By("Creating the secret instance")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"test-data": []byte("test-crt"),
				},
			}

			err = k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the ingress with the secret without crt and key")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameFailure,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("the crt or key does not exist in secret"))

			By("Deleting the wrong secret and creating the correct secret")
			err = k8sClient.Delete(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte("test-crt"),
					"tls.key": []byte("test-key"),
				},
			}

			err = k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the TLS verification failure instance")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameFailure,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TLS verification failed"))

			By("Patching the ingress without the hosts field but with secretName field")
			patch = client.MergeFrom(testIngressFailure.DeepCopy())
			testIngressFailure.Spec.TLS[0].Hosts = []string{}

			err = k8sClient.Patch(ctx, testIngressFailure, patch)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the hosts field does not define in the ingress")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameFailure,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("the Hosts does not define in ingress"))
		})

		It("should successfully reconcile the resource", func() {
			controllerReconciler := &IngressTLSLogReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Interval:             3600,
				IngressErrorMap:      store.NewIngressErrorMap(),
				IngressUpdateTimeMap: store.NewIngressUpdateTimeMap(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedNameSuccess,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
