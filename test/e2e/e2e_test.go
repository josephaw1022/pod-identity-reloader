//go:build e2e
// +build e2e

/*
Copyright 2026 Joseph Whiteaker.

Licensed under the MIT License. See LICENSE for details.
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/josephaw1022/pod-identity-reloader/test/utils"
)

// namespace where the project is deployed in
const namespace = "pod-identity-reloader-system"

// serviceAccountName created for the project
const serviceAccountName = "pod-identity-reloader-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "pod-identity-reloader-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "pod-identity-reloader-metrics-binding"

// controllerDeploymentName is the name of the controller-manager Deployment
// once the config/local-dev/controller kustomize overlay's namePrefix is applied.
const controllerDeploymentName = "pod-identity-reloader-controller-manager"

// ministackNamespace/ministackDeploymentName match config/local-dev/ministack.
const ministackNamespace = "ministack-system"
const ministackDeploymentName = "ministack"

// sampleWorkloadManifest is applied to exercise the controller's core
// reconciliation path (see hack/local-dev/env.sh for the namespace/service
// account it must match, which hack/local-dev/seed-aws.sh seeds a Pod
// Identity Association for).
const sampleWorkloadManifest = "test/e2e/testdata/sample-workload.yaml"
const sampleWorkloadNamespace = "default"
const sampleDeploymentName = "pod-identity-reloader-sample"

// roleARNHashAnnotation and restartedAtAnnotation are written on the pod
// template by the controller; must match internal/controller constants.
const roleARNHashAnnotation = "pod-identity-reloader.dev/role-arn-hash"
const restartedAtAnnotation = "pod-identity-reloader.dev/restarted-at"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment: create the manager
	// namespace, deploy MiniStack into the cluster as a stand-in AWS/EKS API,
	// deploy the controller pointed at it, and seed MiniStack with a sample
	// IAM role, EKS cluster, and Pod Identity Association.
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

		By("deploying MiniStack")
		cmd = exec.Command("kubectl", "apply", "-k", "config/local-dev/ministack")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy MiniStack")

		By("waiting for MiniStack to become ready")
		cmd = exec.Command("kubectl", "-n", ministackNamespace, "rollout", "status",
			fmt.Sprintf("deployment/%s", ministackDeploymentName), "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "MiniStack did not become ready")

		By("deploying the controller-manager against MiniStack")
		cmd = exec.Command("kubectl", "apply", "-k", "config/local-dev/controller")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("pointing the controller-manager at the locally built image")
		cmd = exec.Command("kubectl", "-n", namespace, "set", "image",
			fmt.Sprintf("deployment/%s", controllerDeploymentName),
			fmt.Sprintf("manager=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to set the controller-manager image")

		By("waiting for the controller-manager rollout to complete")
		cmd = exec.Command("kubectl", "-n", namespace, "rollout", "status",
			fmt.Sprintf("deployment/%s", controllerDeploymentName), "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "The controller-manager rollout did not complete")

		By("seeding MiniStack with a sample IAM role, EKS cluster, and Pod Identity Association")
		cmd = exec.Command("hack/local-dev/with-ministack-portforward.sh", "hack/local-dev/seed-aws.sh")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to seed MiniStack")
	})

	// After all tests have been executed, clean up the sample workload, the
	// controller, MiniStack, and the manager namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("removing the sample workload")
		cmd = exec.Command("kubectl", "delete", "-f", sampleWorkloadManifest, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("kubectl", "delete", "-k", "config/local-dev/controller", "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("undeploying MiniStack")
		cmd = exec.Command("kubectl", "delete", "-k", "config/local-dev/ministack", "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
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

			// The sample workload lives in sampleWorkloadNamespace, not
			// namespace (the controller's own namespace), so it needs its
			// own diagnostics dump to debug failures around it.
			By("Fetching sample Deployment state")
			cmd = exec.Command("kubectl", "get", "deployment", sampleDeploymentName,
				"-n", sampleWorkloadNamespace, "-o", "yaml")
			sampleDeploymentOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Sample Deployment:\n%s", sampleDeploymentOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get sample Deployment: %s", err)
			}

			By("Fetching sample workload namespace events")
			cmd = exec.Command("kubectl", "get", "events", "-n", sampleWorkloadNamespace, "--sort-by=.lastTimestamp")
			sampleEventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Sample workload namespace events:\n%s", sampleEventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get sample workload namespace events: %s", err)
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
				By("getting the name of the controller-manager pod")
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

				By("validating the pod's status")
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
				"--clusterrole=pod-identity-reloader-metrics-reader",
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
							"args": [
								"for i in $(seq 1 30); do curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics && exit 0 || sleep 2; done; exit 1"
							],
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

		It("should trigger a rollout when the Pod Identity Association's role changes", func() {
			By("applying the sample workload")
			cmd := exec.Command("kubectl", "apply", "-f", sampleWorkloadManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply the sample workload")

			By("waiting for the controller to write the initial role-arn-hash annotation")
			var initialHash string
			Eventually(func(g Gomega) {
				hash, err := sampleDeploymentAnnotation(roleARNHashAnnotation)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hash).NotTo(BeEmpty(), "expected role-arn-hash annotation to be set")
				initialHash = hash
			}, 2*time.Minute).Should(Succeed())

			By("rotating the IAM role bound via the Pod Identity Association")
			cmd = exec.Command("hack/local-dev/with-ministack-portforward.sh", "hack/local-dev/rotate-sample-role.sh")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to rotate the sample role")

			By("waiting for the controller to detect the role change and trigger a rollout")
			Eventually(func(g Gomega) {
				hash, err := sampleDeploymentAnnotation(roleARNHashAnnotation)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hash).NotTo(BeEmpty(), "expected role-arn-hash annotation to stay non-empty after role rotation")
				g.Expect(hash).NotTo(Equal(initialHash), "expected role-arn-hash annotation to change after role rotation")
			}, 2*time.Minute).Should(Succeed())
		})

		It("should trigger a rollout when the Pod Identity Association is removed", func() {
			beforeRemovalRestartedAt := waitForSampleRestartedAt()

			By("deleting the Pod Identity Association in MiniStack")
			cmd := exec.Command(
				"hack/local-dev/with-ministack-portforward.sh",
				"hack/local-dev/delete-sample-association.sh",
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete the sample Pod Identity Association")

			By("waiting for the controller to detect the removed association and trigger a rollout")
			Eventually(func(g Gomega) {
				hash, err := sampleDeploymentAnnotation(roleARNHashAnnotation)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hash).To(BeEmpty(), "expected role-arn-hash annotation to be cleared after association removal")

				restartedAt, err := sampleDeploymentAnnotation(restartedAtAnnotation)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(restartedAt).NotTo(BeEmpty())
				g.Expect(restartedAt).NotTo(Equal(beforeRemovalRestartedAt),
					"expected restarted-at annotation to be updated after association removal")
			}, 2*time.Minute).Should(Succeed())
		})

		It("should trigger a rollout when a Pod Identity Association is added", func() {
			beforeAddRestartedAt := waitForSampleRestartedAt()

			By("re-creating the Pod Identity Association in MiniStack")
			cmd := exec.Command("hack/local-dev/with-ministack-portforward.sh", "hack/local-dev/seed-aws.sh")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to re-seed the sample Pod Identity Association")

			By("waiting for the controller to detect the new association and trigger a rollout")
			Eventually(func(g Gomega) {
				hash, err := sampleDeploymentAnnotation(roleARNHashAnnotation)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hash).NotTo(BeEmpty(), "expected role-arn-hash annotation to be set after association is added")

				restartedAt, err := sampleDeploymentAnnotation(restartedAtAnnotation)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(restartedAt).NotTo(BeEmpty())
				g.Expect(restartedAt).NotTo(Equal(beforeAddRestartedAt),
					"expected restarted-at annotation to be updated after association is added")
			}, 2*time.Minute).Should(Succeed())
		})
	})
})

// waitForSampleRestartedAt waits for the sample Deployment's restarted-at
// annotation to be populated and returns its value.
func waitForSampleRestartedAt() string {
	var current string
	Eventually(func(g Gomega) {
		restartedAt, err := sampleDeploymentAnnotation(restartedAtAnnotation)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(restartedAt).NotTo(BeEmpty(), "expected restarted-at annotation to be set")
		current = restartedAt
	}, 2*time.Minute).Should(Succeed())
	return current
}

// sampleDeploymentAnnotation returns the value of the given annotation on
// the sample Deployment's pod template.
//
// Dots in key must be escaped: kubectl's JSONPath engine (k8s.io/client-go's
// util/jsonpath) still splits on unescaped "." inside a bracket-quoted
// key, so an expression like annotations['a.b/c'] is treated as a lookup
// for the field "a" (not found) rather than the literal key "a.b/c". With
// kubectl's default AllowMissingKeys behavior this fails silently, printing
// an empty string instead of an error, which would otherwise be very hard
// to notice.
func sampleDeploymentAnnotation(key string) (string, error) {
	escapedKey := strings.ReplaceAll(key, ".", `\.`)
	cmd := exec.Command("kubectl", "get", "deployment", sampleDeploymentName, "-n", sampleWorkloadNamespace,
		"-o", fmt.Sprintf("jsonpath={.spec.template.metadata.annotations['%s']}", escapedKey))
	return utils.Run(cmd)
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	By("creating temporary file to store the token request")
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		By("executing kubectl command to create the token")
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		By("parsing the JSON output to extract the token")
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
