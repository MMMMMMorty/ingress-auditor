/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// IngressTLSLogReconciler reconciles a IngressTLSLog object
type IngressTLSLogReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	ingressMap map[string]error
}

// +kubebuilder:rbac:groups=ingress-audit.morty.dev,resources=ingresstlslogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ingress-audit.morty.dev,resources=ingresstlslogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ingress-audit.morty.dev,resources=ingresstlslogs/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the IngressTLSLog object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *IngressTLSLogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the ingress instance
	ingress := &networkingv1.Ingress{}
	err := r.Get(ctx, req.NamespacedName, ingress)
	if err != nil {
		log.Error(err, "unable to fetch ingress")
		return ctrl.Result{}, fmt.Errorf("unable to fetch ingress, err: %v", err)
	}

	// Check if TLS exists
	// If yes
	if len(ingress.Spec.TLS) != 0 {
		// Check if TLS secret exists
		for _, tls := range ingress.Spec.TLS {
			// Get the secretName
			if tls.SecretName == "" {
				log.Info("The secretName does not define in ingress")
				// generate a log, "namespace-ingress-TLS secretName is empty."
				return ctrl.Result{}, fmt.Errorf("the secretName does not define in ingress")
			}

			// Fetch the secret
			secret := &v1.Secret{}
			err = r.Get(ctx, types.NamespacedName{Name: tls.SecretName, Namespace: ingress.Namespace}, secret)
			if err != nil {
				log.Error(err, "unable to fetch secret "+tls.SecretName)
				return ctrl.Result{}, fmt.Errorf("unable to fetch secret %s in %s, err: %v", tls.SecretName, ingress.Namespace, err)
			}

			// Get crt and key
			crt := secret.Data["tls.crt"]
			key := secret.Data["tls.key"]

			if crt == nil || key == nil {
				log.Info("The crt or key does not exist in secret")
				return ctrl.Result{}, fmt.Errorf("the crt or key does not exist in secret, and the secret is %v", secret)
			}

			// Get the hosts
			if len(tls.Hosts) == 0 {
				log.Info("The Hosts does not define in ingress")
				return ctrl.Result{}, fmt.Errorf("the Hosts does not define in ingress")
			}

			// For all the hosts, use openssl verifies it
			for _, host := range tls.Hosts {
				err = checkTLS(crt, key, host)
				if err != nil {
					log.Error(err, "TLS verification failed")
					return ctrl.Result{}, fmt.Errorf("TLS verification failed, err: %v", err)
				}
			}

			log.Info(fmt.Sprintf("Ingress %s TLS ia applied correctly", req.NamespacedName))
		}
	} else {
		// If not, check if redirect exist.
		if len(ingress.Annotations) == 0 {
			log.Info("TLS is not used and redirect is not applied neither")
			return ctrl.Result{}, fmt.Errorf("TLS is not used and redirect is not applied neither")
		}

		redirect := false
		for key := range ingress.Annotations {
			if strings.Contains(key, "permanent-redirect") || strings.Contains(key, "temporary-redirect") {
				redirect = true
				break
			}
			// TODO: add configuration-snippet, when the value contains 301|302
		}

		if !redirect {
			log.Info("TLS is not used and redirect is not applied neither")
			return ctrl.Result{}, fmt.Errorf("TLS is not used and redirect is not applied neither")
		}

		log.Info(fmt.Sprintf("Ingress %s TLS is not used but redirect is applied", req.NamespacedName))
	}
	// If does not exist, check if not exist in the map, generate one error crd for that ingress
	// Add it to map
	// If exists, continue

	return ctrl.Result{}, nil
}

// checkTLS used the []byte PEM crt and PEM key to connect host with HTTPS
func checkTLS(crtPEM, keyPEM []byte, host string) error {

	// Load client certificate
	cert, err := tls.X509KeyPair(crtPEM, keyPEM)
	if err != nil {
		panic(fmt.Errorf("failed to load cert/key: %v", err))
	}

	// Root CA pool
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(crtPEM)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      rootCAs,
		ServerName:   host, // important: SNI
	}

	conn, err := tls.Dial("tcp", host+":443", tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %v", host, err)
	}
	defer conn.Close()

	return nil
}

// SetupWithManager sets up the controller with the Manager.
// Monitors the ingress
func (r *IngressTLSLogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Named("ingresstlslog").
		Complete(r)
}
