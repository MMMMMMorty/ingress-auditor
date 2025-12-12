# bash
for i in {1..5}; do
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

# kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml

for i in {1..5}; do
    # create ingress
    kubectl apply -f ingresses/ns-$i-ingress.yaml
    ADDRESS=$(kubectl get ing ingress-$i -n ns-$i -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
    sudo sed -i "/[[:space:]]$i$/d" /etc/hosts && echo "$ADDRESS https-example-$i.foo.com" | sudo tee -a /etc/hosts
done

kubectl get ing ingress-2 -n ns-2