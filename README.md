# ingress-auditor
A project that apply a simple Kubernetes operator monitoring ingresses across namespaces. If it finds any ingress in the cluster that isn't secured by a TLS certificate, it logs an error.

## Requirements

1. Monitor Ingress resources across namespaces and log abnormalities when TLS is missing.
2. Allow HTTP endpoints without error logs if the endpoint only returns redirects (HTTP 301/302).
3. Log each issue at most once per configurable interval.
4. preserve the issue state across operator restarts.

## Architecture

A Kubernetes operator that monitors ingress resources and generates an Ingress TLS log CRD when TLS is misconfigured or not properly used. The design of the IngressTLSLog custom resource and its controller is introduced below.

### Structure

```
.devcontainer
   |-- devcontainer.json
   |-- post-install.sh
.dockerignore
.github
   |-- workflows
   |   |-- lint.yml
   |   |-- test-e2e.yml
   |   |-- test.yml
.gitignore
.golangci.yml
Dockerfile
Makefile
PROJECT
README.md
api
   |-- v1alpha1
   |   |-- groupversion_info.go
   |   |-- ingresstlslog_types.go
   |   |-- zz_generated.deepcopy.go
assets
   |-- code_logic.png
cmd
   |-- main.go
config
   |-- crd
   |   |-- bases
   |   |   |-- ingress-audit.morty.dev_ingresstlslogs.yaml
   |   |-- kustomization.yaml
   |   |-- kustomizeconfig.yaml
   |-- default
   |   |-- cert_metrics_manager_patch.yaml
   |   |-- kustomization.yaml
   |   |-- manager_metrics_patch.yaml
   |   |-- metrics_service.yaml
   |-- manager
   |   |-- kustomization.yaml
   |   |-- manager.yaml
   |-- network-policy
   |   |-- allow-metrics-traffic.yaml
   |   |-- kustomization.yaml
   |-- prometheus
   |   |-- kustomization.yaml
   |   |-- monitor.yaml
   |   |-- monitor_tls_patch.yaml
   |-- rbac
   |   |-- ingresstlslog_admin_role.yaml
   |   |-- ingresstlslog_editor_role.yaml
   |   |-- ingresstlslog_viewer_role.yaml
   |   |-- kustomization.yaml
   |   |-- leader_election_role.yaml
   |   |-- leader_election_role_binding.yaml
   |   |-- metrics_auth_role.yaml
   |   |-- metrics_auth_role_binding.yaml
   |   |-- metrics_reader_role.yaml
   |   |-- role.yaml
   |   |-- role_binding.yaml
   |   |-- service_account.yaml
   |-- samples
   |   |-- ingress-audit_v1alpha1_ingresstlslog.yaml
   |   |-- kustomization.yaml
go.mod
go.sum
hack
   |-- boilerplate.go.txt
internal
   |-- controller
   |   |-- ingresstlslog_controller.go
   |   |-- ingresstlslog_controller_test.go
   |   |-- suite_test.go
   |-- store
   |   |-- ingress_error_map.go
   |   |-- ingress_update_time_map.go
   |-- utils
   |   |-- tls.go
local_test
   |-- create_and_deploy.sh
   |-- create_ingress.sh
   |-- dns
   |   |-- coredns-original.yaml
   |-- ingresses
   |   |-- ns-1-ingress.yaml
   |   |-- ns-2-ingress.yaml
   |   |-- ns-3-ingress.yaml
   |   |-- ns-4-ingress.yaml
   |   |-- ns-5-ingress.yaml
   |   |-- ns-6-ingress.yaml
   |   |-- ns-7-ingress.yaml
   |   |-- ns-8-ingress.yaml
   |-- san-5.conf
   |-- san-8.conf
   |-- tls-2.crt
   |-- tls-2.key
   |-- tls-3.crt
   |-- tls-3.key
   |-- tls-5.crt
   |-- tls-5.key
   |-- tls-8.crt
   |-- tls-8.key
test
   |-- e2e
   |   |-- e2e_suite_test.go
   |   |-- e2e_test.go
   |   |-- ingresses
   |   |   |-- ns-1-ingress.yaml
   |   |   |-- ns-2-ingress.yaml
   |   |   |-- ns-3-ingress.yaml
   |   |   |-- ns-4-ingress.yaml
   |   |   |-- ns-5-ingress.yaml
   |   |   |-- ns-6-ingress.yaml
   |   |   |-- ns-7-ingress.yaml
   |   |   |-- ns-8-ingress.yaml
   |   |-- tls
   |   |   |-- tls-2.crt
   |   |   |-- tls-2.key
   |   |   |-- tls-3.crt
   |   |   |-- tls-3.key
   |   |   |-- tls-5.crt
   |   |   |-- tls-5.key
   |   |   |-- tls-8.crt
   |   |   |-- tls-8.key
   |-- utils
   |   |-- dns.go
   |   |-- utils.go
```

### IngressTLSLog Custom Resource

This resource is used to persist logs when ingress TLS is not properly configured or used. A [sample](config/samples/ingress-audit_v1alpha1_ingresstlslog.yaml) is shown below.

```
apiVersion: ingress-audit.morty.dev/v1alpha1
kind: IngressTLSLog
metadata:
  labels:
    app.kubernetes.io/name: ingress-auditor
    app.kubernetes.io/managed-by: kustomize
  name: ingresstlslog-sample
spec:
  generationTimestamp: "2025-12-12T00:00:00Z"
  ingressName: ingress-example
  level: Error
  message: the secretName does not define in ingress
  namespace: ns-example
```
- `generationTimestamp`: the generation time of the log
- `ingressName`: the name of the ingress
- `namespace`:  the namespace of the ingress
- `level`: the log severity, including `Error`, `Warn` and `Info`
- `message`: the log

Generated CRD name rule: `<namespace>-<ingressName>-<generationTimestamp>-<four random number>`

### Controller

This controller is designed to satisfy the four requirements described above.

For requirements 1 and 2, the design is shown in the below figure.

![Basic design](assets/code_logic.png)

There are eight types of errors, each mapped to a different error message in the CRD:
- `ErrFetchIngress` : "unable to fetch ingress"
- `ErrSecretNameMissing`: "the secretName does not define in ingress"
- `ErrFetchSecret`: "unable to fetch secret"
- `ErrCrtOrKeyMissing`: "the crt or key does not exist in secret"
- `ErrHostsMissing`: "the Hosts does not define in ingress"
- `ErrTLSVerification`: "TLS verification failed"
- `ErrHTTPRedirectMissing`: "TLS is not used and redirect is not applied neither"
- `ErrCreateTLSLog`: "failed to create new TLS log"


For requirement 3, 

A flag `interval-second` ia introduced to enable user to set interval in seconds, in default is 3600. Then, `RequeueAfter: Interval` is used to request controller retry after interval time.

`IngressUpdateTimeMap`, a mapping between namespace_ingress and log last update time, is introduced in controller to record the last update time to enable update when the last update time + interval < now.

`IngressErrorMap`, a mapping between namespace_ingress and error type, is introduced to check if the last time error type is the same as this time, if yes, then do not log; if not, log the new err.

Noted: **Even though the error type is the same, the last update time + interval < now, it still logs the error.**

For requirement 4, 

The IngressTLSLog CRD is generated when an error is detected. Its purpose is to persist the error log. The CRD is owned by the ingress that it generated from, which means if the ingress is deleted, the log will be deleted. In special case, during log generation, ingress is not fetched, then the log will be generated in `ingress-auditor-system` (It will not disappear even the ingress is deleted).

## Development

### Development Guide
1. Set up instance(minimal images), connect with ssh key
2. Add a new user (su - morty) and run `sudo apt install build-essential` to install make
3. Install [Docker](https://docs.docker.com/engine/install/ubuntu/) and add user to docker group
4. Set up Git in instance, generate git key, pull repo from Git
5. Install [Minikube](https://minikube.sigs.k8s.io/docs/start/?arch=%2Flinux%2Fx86-64%2Fstable%2Fbinary+download) (fast k8s setup) and start
6. Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/)
7. Install latest [Go-1.24.11](https://go.dev/doc/install)
8. Install [golangci-lint](https://golangci-lint.run/docs/welcome/install/)
9. Choose Operator Framework or non-framework? -> framework (easier to develop and less security vunlerbility) kubebuilder 
10. Install [kubeBuilder](https://book.kubebuilder.io/quick-start.html#installation)
11. Init kubebuilder project and API
  ```
  kubebuilder init \
    --domain morty.dev \
    --repo github.com/MMMMMMorty/ingress-auditor

  kubebuilder create api \
      --group ingress-audit \
      --version v1alpha1 \
      --kind IngressTLSLog
  ```
  The generated api is ingress-audit.morty.dev/v1alpha1

12. Define ingress-audit CR and controller

Modify the files  for [ingress-audit CR](api/v1alpha1/ingresstlslog_types.go) and [controller](internal/controller/ingresstlslog_controller.go)

After changes, apply below commands:
```
make generate
make manifests
```
Both commands use controller-gen with different flags for code and manifest generation, respectively.

13. Update [controller test](internal/controller/ingresstlslog_controller_test.go) and [e2e test](test/e2e/e2e_test.go)
14. Replace Kind with Minikube in Actions
15. Fix linter error

### Generate the Required Files

Create the DeepCopy implementations 
```
make generate
```

Generate the CRD manifests
```
make manifests
```

### Deploy (Local Cluster: Minikube)
```
docker login
```

Build and push your image to the location specified by IMG:
```
make docker-build docker-push IMG=mmmmmmorty/ingress-auditor:<version>
```
Deploy the controller to the cluster with image specified by IMG:
```
make deploy IMG=mmmmmmorty/ingress-auditor:<version>
```

### Debugging Kubectl Commands

Get all the resourses of ingress-auditor
```
kubectl get all -n ingress-auditor-system
```

Check the log of pod of ingress-auditor
```
kubectl logs <pod> -n ingress-auditor-system
```
Check DNS records
```
kubectl get configmap coredns -n kube-system -oyaml
```

Check the generated ingress TLS log

Usually the ingressTLSlog is generated under the same namespace as ingress
```
kubectl get ingresstlslogs.ingress-audit.morty.dev -n <ingress-namespace>
```
However, if the error is `ErrFetchIngress` ("unable to fetch ingress"), the log is generated in `ingress-auditor-system` namespace.
```
kubectl get ingresstlslogs.ingress-audit.morty.dev -n ingress-auditor-system
```
Restart the deployments/pods of ingress-auditor
```
kubectl rollout restart deployment -n ingress-auditor-system
```

Test the TLS connectivity example
```
kubectl run tls-test -n ns-5\
  --image=curlimages/curl \
  --restart=Never \
  --overrides='
{
  "apiVersion":"v1",
  "spec":{
    "volumes":[
      {"name":"tls","secret":{"secretName":"secret-tls"}}
    ],
    "containers":[{
      "name":"curl",
      "image":"curlimages/curl",
      "command":["sleep","3600"],
      "volumeMounts":[{"name":"tls","mountPath":"/tls"}]
    }]
  }
}'
kubectl exec -it tls-test -n ns-5 -- curl -v https://https-example-5.foo.com --cacert /tls/tls.crt 
```

### Remove (Local Cluster)

```
make undeploy
make uninstall
```

## Test

### Local Cluster Test

Requirements:
- minikube v1.37.0
- Go 1.24.11

Start minikube, enable ingress-nginx, clean env, generate file and deploy:
```
cd local_test
bash create_and_deploy.sh <version_number> (recommand to start with 200)
```

Noted: **enable ingress-nginx is necessary. Since ingress does not contain address, TLS verification will fail**

Deploy testing resources: 
```
bash create_ingress.sh
```

Check the results:
```
kubectl logs <pod> -n ingress-auditor-system
```

5 types of errors are tested, and 2 types of successful cases. More complete tests are in controller test(below). Here are the relationships:

```
"ingress-1": ErrFetchSecret,
"ingress-2": ErrTLSVerification,
"ingress-3": ErrTLSVerification,
"ingress-4": ErrSecretNameMissing,
"ingress-5": Success, (HTTPS)
"ingress-6": Success, (HTTP)
"ingress-7": ErrHTTPRedirectMissing,
"ingress-8": ErrHostsMissing
```

#### Test case generation

SSL generate key and crt, generate key + self-signed cert in one command with pre-defined config
```
sudo openssl req -x509 -newkey rsa:2048 \
  -keyout <keyName>.key \
  -out <crtName>.crt \
  -days 365 \
  -nodes \
  -config <configName>.conf
```

Generate secret
```
kubectl create secret tls secret-tls \
  --cert=<crtName>.crt \
  --key=<keyName>.key \
  -n <ns>
```

Define your own service, then ingress, use the above secret as secret name in TLS.

### Controller Test

Controller Test can be tested locally or in Github Action.
```
make test
```

This controller test contains 8 types of error failure test and one successful test (Non HTTP + redirect)

### E2E Tests
TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated, temporary environment to validate project changes with the purpose of being used in CI jobs. The default setup requires Kind, builds/loads the Manager Docker image locally, and installs CertManager. E2E tests takes 5-15 mins. The timeout is set as 30 mins.
```
make test-e2e
```
E2E tests run the same test as local test with the same resources.

remove the e2e test
```
make cleanup-test-e2e
```
## Debugging problem
`Error: failed to create API: unable to run post-scaffold tasks of "base.go.kubebuilder.io/v4": exit status 2`

[Solution](https://github.com/operator-framework/operator-sdk/issues/6681)

## Reference

### Website
- [how to achieve redirect 301|300 in ingress](https://stackoverflow.com/questions/53518739/kubernetes-nginx-ingress-configmap-301-redirect)
- [Watching Secondary Resources Owned by the Controller](https://book.kubebuilder.io/reference/watching-resources/secondary-owned-resources?search=SetControllerReference)
- [requeueafter-x](https://book.kubebuilder.io/reference/watching-resources.html?highlight=RequeueAfter#when-requeueafter-x-is-useful)
- [kubebuilder Get Started](https://book.kubebuilder.io/getting-started#sample-of-custom-resources)
- [Is there a way to represent a directory tree in a Github README.md](https://stackoverflow.com/questions/23989232/is-there-a-way-to-represent-a-directory-tree-in-a-github-readme-md)
## LLM
chatgpt prompts:
- [in k8s ingress how to check if TLS works?](https://chatgpt.com/s/t_69397b3fa35c8191a41c4338b1a80c32)
- [I just want to try if the connection work, if not, err, if yes, continue, next host](https://chatgpt.com/s/t_69398ddb3bf88191951566a6a951c04e)
- [SSL generation, gen key and gen crt with conf](https://chatgpt.com/s/t_693d45b0188c81919645343119cd08df)
- [TLS connection with base64 string crt and key?](https://chatgpt.com/s/t_693a52439b788191adc640a004ec6917)
- [User can only input number as second, then change it to time.Duration](https://chatgpt.com/s/t_693a870d19108191811e8b2c9f12a625)
- [how to use time.Now() compare lastupdateTIme time.Time + interval Time.Duration](https://chatgpt.com/s/t_693a892da3c88191ad117fd27c621e0f)
- [for kubebuilder, how to know Reconcile is triggered by event or shcedule time (requeueAfter)?](https://chatgpt.com/s/t_693a86f3dbf08191b227562dff37f26a)
- [how to use time.Now() compare lastupdateTIme time.Time + interval Time.Duration](https://chatgpt.com/s/t_693a892da3c88191ad117fd27c621e0f)
- [how to changec time.Time to *metav1.Time](https://chatgpt.com/s/t_693a94daf7c88191ba77e7e73076a7e7)
- [write e2e tests to create the env](https://chatgpt.com/share/693ac4c7-0108-8011-883a-24785038af88)
- [create ingresses that only return redirects (HTTP 301/302)](https://chatgpt.com/s/t_6938eb6cee2c8191adc9e71031c95a49)
- [try if the connection work, if not, err, if yes, continue, next host]()
- [Create mermaid code with this logic](https://chatgpt.com/s/t_693c3c56550c8191a23b659fd18acc03)

## Thinking

- Why use ownership? 

  Answer: Ingress owns ingresstlslog, so the ingresstlslog can be deleted when ingress is deleted. It is noted that in the begining version, ingresstlslog is designed to be generated only in `ingress-auditor-system` namespace. In this case, cross-namespace owner references are disallowed.

- Why choose minkube over kind?

  Answer: for TLS verification test, ingress with address is needed, when using kind, ingress ADDRESS will always be empty unless you install MetalLB. For minikube, the solution is eaiser.

- Why do I choose framework over non-framework, and why do I choose Kubebuilder

Non-framework: high control, flexible, adjustable, but I need to handle a lot of things myself. If I make a mistake in any part, it can cause vulnerability.
Framework: procvide scaffolding, tooling, and established patterns that accelerate development, manage boilerplate, and ensure best practices.

Since I am doing a simple project, a framework will make my job eaiser.

Two mainstream operators:

operator-sdk: Multiple languages, support for Ansible or Helm. Focus on full-lifecycle of operator.

kubebuilder: Primarily Golang and Kubernetes native based. Can be extensively used in operator-sdk.

They both make use of controller-runtime and have the same basic layout. Consider I want to use golang to create a simple operator for specific function, kubebuilder seems to be a better option.

- Why the local test can pass but the CI test for test case 5 does not work?

After several debugging (not trusted CA, DNS problem), I located the problem to: using `sudo` to write the ip resolve line to host machine, since CI environments don’t allow sudo or host-level changes, so modifying /etc/hosts fails.

I decided to change host DNS to CoreDNS, which operates entirely in user space and inside the CI environment. So I converts a CoreDNS Corefile into JSON, patches the Kubernetes ConfigMap, and restarts CoreDNS to make sure new DNS rules take effect.
