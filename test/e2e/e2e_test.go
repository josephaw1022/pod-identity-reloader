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
	"net"
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

// localstackHostPort/kindDockerNetwork must match hack/local-dev/env.sh's
// LOCALSTACK_HOST_PORT/KIND_DOCKER_NETWORK defaults (overridable via the
// same-named env vars, mirroring the shell tooling).
const defaultLocalstackHostPort = "4566"
const defaultKindDockerNetwork = "kind"

// sampleWorkloadManifest is applied to exercise the controller's core
// reconciliation path (see hack/local-dev/env.sh for the namespace/service
// account it must match, which hack/local-dev/seed-aws.sh seeds a Pod
// Identity Association for).
const sampleWorkloadManifest = "test/e2e/testdata/sample-workload.yaml"
const sampleWorkloadNamespace = "default"
const sampleDeploymentName = "pod-identity-reloader-sample"

// roleARNHashAnnotation is written on the pod template by the controller;
// must match internal/controller.RoleARNHashAnnotation.
const roleARNHashAnnotation = "pod-identity-reloader.dev/role-arn-hash"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment: create the manager
	// namespace, start LocalStack as a stand-in AWS/EKS API, deploy the
	// controller pointed at it, and seed LocalStack with a sample IAM role,
	// EKS cluster, and Pod Identity Association.
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

		By("starting LocalStack (requires LOCALSTACK_AUTH_TOKEN for EKS Pod Identity emulation)")
		cmd = exec.Command("hack/local-dev/localstack-up.sh")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to start LocalStack")

		By("deploying the controller-manager")
		cmd = exec.Command("kubectl", "apply", "-k", "config/local-dev/controller")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("pointing the controller-manager at LocalStack")
		localstackEndpoint, err := localstackKindEndpoint()
		Expect(err).NotTo(HaveOccurred(), "Failed to resolve the LocalStack endpoint reachable from Kind")
		cmd = exec.Command("kubectl", "-n", namespace, "set", "env",
			fmt.Sprintf("deployment/%s", controllerDeploymentName),
			fmt.Sprintf("AWS_ENDPOINT_URL=%s", localstackEndpoint))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to set the controller-manager's AWS_ENDPOINT_URL")

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

		By("seeding LocalStack with a sample IAM role, EKS cluster, and Pod Identity Association")
		cmd = exec.Command("hack/local-dev/seed-aws.sh")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to seed LocalStack")
	})

	// After all tests have been executed, clean up the sample workload, the
	// controller, LocalStack, and the manager namespace.
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

		By("stopping LocalStack")
		cmd = exec.Command("hack/local-dev/localstack-down.sh")
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
			cmd = exec.Command("hack/local-dev/rotate-sample-role.sh")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to rotate the sample role")

			By("waiting for the controller to detect the role change and trigger a rollout")
			Eventually(func(g Gomega) {
				hash, err := sampleDeploymentAnnotation(roleARNHashAnnotation)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hash).NotTo(Equal(initialHash), "expected role-arn-hash annotation to change after role rotation")
			}, 2*time.Minute).Should(Succeed())
		})
	})
})

// localstackKindEndpoint resolves the AWS_ENDPOINT_URL a pod running inside
// Kind must use to reach the host-level LocalStack container started by
// hack/local-dev/localstack-up.sh. LocalStack publishes its port on every
// host interface, including the kind Docker network's gateway, so pods can
// reach it there without LocalStack joining any special network itself.
func localstackKindEndpoint() (string, error) {
	kindDockerNetwork := os.Getenv("KIND_DOCKER_NETWORK")
	if kindDockerNetwork == "" {
		kindDockerNetwork = defaultKindDockerNetwork
	}
	localstackHostPort := os.Getenv("LOCALSTACK_HOST_PORT")
	if localstackHostPort == "" {
		localstackHostPort = defaultLocalstackHostPort
	}

	ipamConfigs, err := dockerNetworkIPAMConfigs(kindDockerNetwork)
	if err != nil {
		return "", err
	}

	ipamConfig, err := firstIPv4IPAMConfig(ipamConfigs)
	if err != nil {
		return "", fmt.Errorf("docker network %q: %w", kindDockerNetwork, err)
	}

	gatewayIP := ipamConfig.Gateway
	if gatewayIP == "" {
		// Docker sometimes reports an empty Gateway even for a fully
		// functional network: the daemon populates it lazily and, on
		// ephemeral CI runners, it isn't always resolved by the time this
		// runs (see moby/moby#51890 and moby/moby#26799). Fall back to
		// deriving the gateway from the network's subnet: Docker always
		// assigns the first usable address in the subnet as the gateway.
		gatewayIP, err = gatewayFromSubnet(ipamConfig.Subnet)
		if err != nil {
			return "", fmt.Errorf("docker network %q reported no gateway IP and it could not be derived from its subnet: %w", kindDockerNetwork, err)
		}
	}

	return fmt.Sprintf("http://%s:%s", gatewayIP, localstackHostPort), nil
}

// ipamConfig mirrors the subset of Docker's network IPAM config this file
// needs (docker network inspect .IPAM.Config entries).
type ipamConfig struct {
	Subnet  string `json:"Subnet"`
	Gateway string `json:"Gateway"`
}

// dockerNetworkIPAMConfigs returns every IPAM config block (one per IP
// family) for the given Docker network.
func dockerNetworkIPAMConfigs(dockerNetwork string) ([]ipamConfig, error) {
	cmd := exec.Command("docker", "network", "inspect", dockerNetwork,
		"--format", "{{ json .IPAM.Config }}")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("inspecting docker network %q: %w", dockerNetwork, err)
	}

	var configs []ipamConfig
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &configs); err != nil {
		return nil, fmt.Errorf("parsing IPAM config for docker network %q: %w", dockerNetwork, err)
	}

	return configs, nil
}

// firstIPv4IPAMConfig returns the first IPv4 IPAM config in configs. Kind's
// Docker network is dual-stack, so blindly picking configs[0] can select an
// IPv6 entry, producing an endpoint the AWS SDK can't dial.
func firstIPv4IPAMConfig(configs []ipamConfig) (ipamConfig, error) {
	for _, c := range configs {
		_, ipNet, err := net.ParseCIDR(c.Subnet)
		if err != nil {
			continue
		}
		if ipNet.IP.To4() != nil {
			return c, nil
		}
	}
	return ipamConfig{}, fmt.Errorf("no IPv4 IPAM config found")
}

// gatewayFromSubnet derives the Docker-assigned gateway address for the
// given subnet by computing its first usable host address, which is how
// Docker's default bridge driver allocates gateways.
func gatewayFromSubnet(subnet string) (string, error) {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", fmt.Errorf("parsing subnet %q: %w", subnet, err)
	}

	gatewayIP := ip.Mask(ipNet.Mask)
	incrementIP(gatewayIP)

	return gatewayIP.String(), nil
}

// incrementIP increments ip in place by one address, e.g. 172.18.0.0 becomes
// 172.18.0.1.
func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

// sampleDeploymentAnnotation returns the value of the given annotation on
// the sample Deployment's pod template.
func sampleDeploymentAnnotation(key string) (string, error) {
	cmd := exec.Command("kubectl", "get", "deployment", sampleDeploymentName, "-n", sampleWorkloadNamespace,
		"-o", fmt.Sprintf("jsonpath={.spec.template.metadata.annotations['%s']}", key))
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
