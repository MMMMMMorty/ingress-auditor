# bash
cd ..

make uninstall
make manifests
make generate
make docker-build docker-push IMG=mmmmmmorty/ingress-auditor:v0.$1
make deploy IMG=mmmmmmorty/ingress-auditor:v0.$1

