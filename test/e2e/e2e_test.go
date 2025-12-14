//go:build e2e
// +build e2e

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

package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/MMMMMMorty/ingress-auditor/test/utils"
)

// namespace where the project is deployed in
const namespace = "ingress-auditor-system"

// serviceAccountName created for the project
const serviceAccountName = "ingress-auditor-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "ingress-auditor-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "ingress-auditor-metrics-binding"

// namespaceNumber is the number of test namespaces
const namespaceNumber = 8

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("waiting up to 3 minutes for ingress-nginx controller to be ready")
		cmd = exec.Command(
			"kubectl", "wait", "--namespace", "ingress-nginx",
			"--for=condition=available", "--timeout=180s", "deployment/ingress-nginx-controller",
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		// Creates my own environment for testing
		// Create 5 ns and create service expose port 80
		By("creating tetsting namespace")
		for i := 1; i <= namespaceNumber; i++ {
			ns := fmt.Sprintf("ns-%d", i)
			dep := fmt.Sprintf("nginx-%d", i)

			cmd = exec.Command("kubectl", "create", "ns", ns)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

			By("creating deployment " + dep)
			_, err = exec.Command("kubectl", "create", "deployment", dep, "-n", ns, "--image=nginx").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			By("exposing service " + dep)
			_, err = exec.Command("kubectl", "expose", "deployment", dep, "-n", ns, "--port=80", "--target-port=80").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
		}

		// define createSecret function to create TLS secrets
		createSecret := func(ns, certPath, keyPath string) {
			By("creating secret in " + ns)

			cmd = exec.Command(
				"kubectl", "create", "secret", "tls", "secret-tls",
				"--cert="+certPath,
				"--key="+keyPath,
				"-n", ns,
			)

			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		}

		// creates 3 TLS secrets
		createSecret("ns-2", "test/e2e/tls/tls-2.crt", "test/e2e/tls/tls-2.key")
		createSecret("ns-3", "test/e2e/tls/tls-3.crt", "test/e2e/tls/tls-3.key")
		createSecret("ns-5", "test/e2e/tls/tls-5.crt", "test/e2e/tls/tls-5.key")
		createSecret("ns-8", "test/e2e/tls/tls-8.crt", "test/e2e/tls/tls-8.key")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)

		By("removing secret")
		// define deleteSecrets function
		deleteSecret := func(ns string) {
			By("deleting secret in " + ns)
			exec.Command("kubectl", "delete", "secret", "secret-tls", "-n", ns).CombinedOutput()
		}

		// delete 3 TLS secrets
		deleteSecret("ns-2")
		deleteSecret("ns-3")
		deleteSecret("ns-5")
		deleteSecret("ns-8")

		By("reset CoreDNS")
		err := utils.ResetCoreDNS()
		Expect(err).NotTo(HaveOccurred(), "Failed to configure CoreDNS")

	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=ingress-auditor-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput, err := getMetricsOutput()
		// Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
		It("test ingresses's failed and successful cases", func() {
			By("Configuring CoreDNS before creating ingresses")
			// In Minikube, all ingresses share the same IP (node IP)
			// Configure DNS first so controller can resolve hostnames immediately
			minikubeIP := os.Getenv("MINIKUBE_IP")
			if minikubeIP == "" {
				// Get Minikube IP using minikube command
				minikubeBinary := os.Getenv("MINIKUBE")
				if minikubeBinary == "" {
					minikubeBinary = "minikube"
				}
				profile := os.Getenv("MINIKUBE_PROFILE")
				if profile == "" {
					profile = "minikube"
				}
				cmd := exec.Command(minikubeBinary, "ip", "-p", profile)
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to get Minikube IP")
				minikubeIP = strings.TrimSpace(output)
			}
			Expect(minikubeIP).NotTo(BeEmpty(), "Minikube IP is empty")

			hostMappings := make(map[string]string)
			for i := 1; i <= namespaceNumber; i++ {
				hostname := fmt.Sprintf("https-example-%d.foo.com", i)
				hostMappings[hostname] = minikubeIP
			}
			err := utils.ConfigureCoreDNS(hostMappings)
			Expect(err).NotTo(HaveOccurred(), "Failed to configure CoreDNS")

			By("Creating 8 ingresses")
			for i := 1; i <= namespaceNumber; i++ {
				cmd := exec.Command("kubectl", "apply", "-f",
					fmt.Sprintf("test/e2e/ingresses/ns-%d-ingress.yaml", i))
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to create ingress %s", output)
				fmt.Printf("ingress is created in ns-%d\n", i)
			}

			By("Waiting for ingresses to get IPs")
			waitForIngress := func(g Gomega) {
				for i := 1; i <= namespaceNumber; i++ {
					cmd := exec.Command(
						"kubectl", "get", "ing", fmt.Sprintf("ingress-%d", i),
						"-n", fmt.Sprintf("ns-%d", i),
						"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}",
					)
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(strings.TrimSpace(output)).NotTo(BeEmpty(), "ingress-%d has no IP", i)
				}
			}
			Eventually(waitForIngress, 5*time.Minute, time.Second).Should(Succeed())

			By("Creating result map")
			// var ErrFetchIngress = errors.New("unable to fetch ingress")
			var ErrSecretNameMissing = errors.New("the secretName does not define in ingress")
			var ErrFetchSecret = errors.New("unable to fetch secret")
			// var ErrCrtOrKeyMissing = errors.New("the crt or key does not exist in secret")
			var ErrHostsMissing = errors.New("the Hosts does not define in ingress")
			var ErrTLSVerification = errors.New("TLS verification failed")
			var ErrHTTPRedirectMissing = errors.New("TLS is not used and redirect is not applied neither")
			// var ErrCreateTLSLog = errors.New("failed to create new TLS log")

			results := map[string]error{
				"ns-1": ErrFetchSecret,
				"ns-2": ErrTLSVerification,
				"ns-3": ErrTLSVerification,
				"ns-4": ErrSecretNameMissing,
				"ns-5": nil,
				"ns-6": nil,
				"ns-7": ErrHTTPRedirectMissing,
				"ns-8": ErrHostsMissing,
			}

			By("Verifying the failure results")
			cmd := exec.Command("kubectl", "get",
				"pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}"+
					"{{ if not .metadata.deletionTimestamp }}"+
					"{{ .metadata.name }}"+
					"{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", namespace,
			)

			podOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
			podNames := utils.GetNonEmptyLines(podOutput)
			Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
			controllerPodName = podNames[0]
			Expect(controllerPodName).To(ContainSubstring("controller-manager"))

			verifyfailure := func(g Gomega) {
				for i := 1; i <= namespaceNumber; i++ {
					if i == 5 || i == 6 {
						continue // manually skip successful cases, which will be verified later
					}
					cmd := exec.Command("sh", "-c",
						fmt.Sprintf("kubectl get ingresstlslogs.ingress-audit.morty.dev -n ns-%d | grep ns-%d-ingress-%d | awk '{print $1}'", i, i, i))
					// kubectl get ingresstlslogs.ingress-audit.morty.dev -n ingress-auditor-system \
					// -o json | jq -r '.items[] | select(.metadata.name | startswith("ns-1-ingress-1")) | .metadata.name'
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred(), "Failed to get ingresstlslogs %s", output)
					g.Expect(output).To(ContainSubstring(fmt.Sprintf("ns-%d-ingress-%d", i, i)))

					tlslog := strings.TrimSpace(output)
					cmd = exec.Command(
						"kubectl", "get", "ingresstlslogs", tlslog,
						"-n", fmt.Sprintf("ns-%d", i),
						"-o", "jsonpath={.spec.message}",
					)
					output, err = utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred(), "Failed to get message from ingresstlslog %s", output)
					errType := results[fmt.Sprintf("ns-%d", i)]
					g.Expect(output).To(ContainSubstring(errType.Error()))
				}
			}

			Eventually(verifyfailure, 10*time.Minute, time.Second).Should(Succeed())

			By("Verifying the successful results")
			lastIndex := 0 // Only read from the new part

			verifyNS5success := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				lines := strings.Split(output, "\n")

				HTTPSCount := 0

				for i := lastIndex; i < len(lines); i++ {
					fmt.Println(lines[i])
					// Count ns-5 once
					if strings.Contains(lines[i], "ns-5/ingress-5 TLS ia applied correctly") {
						HTTPSCount++
						break
					}

				}

				lastIndex = len(lines)

				// One line contains `TLS ia applied correctly``
				g.Expect(HTTPSCount).To(Equal(1))
			}

			Eventually(verifyNS5success, 10*time.Minute, time.Second).Should(Succeed())

			lastIndex = 0 // remember this between loops

			verifyNS6success := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				lines := strings.Split(output, "\n")

				HTTPCount := 0

				for i := lastIndex; i < len(lines); i++ {
					// Count ns-6 once
					if strings.Contains(lines[i], "ns-6/ingress-6 TLS is not used but redirect is applied") {
						HTTPCount++
						break
					}
				}

				lastIndex = len(lines)

				g.Expect(HTTPCount).To(Equal(1))
			}

			Eventually(verifyNS6success, 10*time.Minute, time.Second).Should(Succeed())
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
