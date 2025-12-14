#!/bin/bash
# Make sure the number is the same number as the ns number
for i in {1..8}; do
    kubectl create ns ns-$i
    # create deployment
    kubectl create deployment nginx-$i \
    -n ns-$i \
    --image=nginx
    # expose nginx service
    kubectl expose deployment nginx-$i \
    -n ns-$i \
    --port=80 --target-port=80
done

# recreate secret
kubectl delete secret secret-tls \
  -n ns-2

kubectl create secret tls secret-tls \
  --cert=tls-2.crt \
  --key=tls-2.key \
  -n ns-2

kubectl delete secret secret-tls \
  -n ns-3

kubectl create secret tls secret-tls \
  --cert=tls-3.crt \
  --key=tls-3.key \
  -n ns-3

kubectl delete secret secret-tls \
  -n ns-5

kubectl create secret tls secret-tls \
  --cert=tls-5.crt \
  --key=tls-5.key \
  -n ns-5

kubectl delete secret secret-tls \
  -n ns-8

kubectl create secret tls secret-tls \
  --cert=tls-8.crt \
  --key=tls-8.key \
  -n ns-8

# Collect hostname mappings
HOSTS_ENTRIES=""
for i in {1..8}; do
  # create ingress
  kubectl apply -f ingresses/ns-$i-ingress.yaml
  ADDRESS=$(minikube ip)

  if [ -z "$ADDRESS" ]; then
    echo "ERROR: failed to get Minikube IP"
    exit 1
  fi

  HOSTS_ENTRIES="${HOSTS_ENTRIES}        ${ADDRESS} https-example-${i}.foo.com\n"
done

# Configure CoreDNS with custom host entries
COREFILE=".:53 {
  errors
  health {
      lameduck 5s
  }
  ready
  kubernetes cluster.local in-addr.arpa ip6.arpa {
      pods insecure
      fallthrough in-addr.arpa ip6.arpa
      ttl 30
  }
  hosts {
${HOSTS_ENTRIES}        fallthrough
  }
  prometheus :9153
  forward . /etc/resolv.conf {
      max_concurrent 1000
  }
  cache 30
  loop
  reload
  loadbalance
}
"

# converts a multi-line Corefile into a JSON-safe string
ESCAPED_COREFILE=$(echo "$COREFILE" | sed 's/"/\\"/g' | awk '{printf "%s\\n", $0}' | sed 's/\\n$//')
PATCH_JSON="{\"data\":{\"Corefile\":\"${ESCAPED_COREFILE}\"}}"

echo "Patching CoreDNS ConfigMap..."
kubectl patch configmap coredns -n kube-system --type merge -p "$PATCH_JSON"

# restart CoreDNS to reload config
echo "Restarting CoreDNS..."
kubectl rollout restart deployment/coredns -n kube-system
kubectl rollout status deployment/coredns -n kube-system --timeout=60s

echo "CoreDNS configured with custom host entries"