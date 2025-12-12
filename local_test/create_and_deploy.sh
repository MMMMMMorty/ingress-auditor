# bash
cd ../

# start Minikube
minikube start
# Enable ingress-nginx (Necessary!)
minikube addons enable ingress
# Remove the old ingress auditor
make uninstall
# generate files
make manifests
make generate
# deploy
make docker-build docker-push IMG=mmmmmmorty/ingress-auditor:v0.$1
make deploy IMG=mmmmmmorty/ingress-auditor:v0.$1
