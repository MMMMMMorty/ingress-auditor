# bash
cd ../

# make setup-test-e2e
# kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
# helm install ingress-nginx ingress-nginx/ingress-nginx \
#   --namespace ingress-nginx \
#   --create-namespace

# kubectl wait -n ingress-nginx \
#   --for=condition=available deployment/ingress-nginx-controller \
#   --timeout=120s

# kubectl get pods -n ingress-nginx -l app.kubernetes.io/component=admission-webhook
# kubectl delete validatingwebhookconfiguration ingress-nginx-admission

make uninstall
make manifests
make generate
make docker-build docker-push IMG=mmmmmmorty/ingress-auditor:v0.$1
make deploy IMG=mmmmmmorty/ingress-auditor:v0.$1
