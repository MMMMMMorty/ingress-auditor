# ingress-auditor
A project that apply a simple Kubernetes operator monitoring ingresses across namespaces. If it finds any ingress in the cluster that isn't secured by a TLS certificate, it logs an error.

# Steps
1. Set up instance, connect with ssh key
2. Set up Git in instance, generate git key, pull repo from Git
3. Install [Minikube](https://minikube.sigs.k8s.io/docs/start/?arch=%2Flinux%2Fx86-64%2Fstable%2Fbinary+download) (fast k8s setup) and start 