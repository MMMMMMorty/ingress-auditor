# ingress-auditor
A project that apply a simple Kubernetes operator monitoring ingresses across namespaces. If it finds any ingress in the cluster that isn't secured by a TLS certificate, it logs an error.
