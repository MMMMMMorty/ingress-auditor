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
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ingressauditv1alpha1 "github.com/MMMMMMorty/ingress-auditor/api/v1alpha1"
)

// IngressTLSLogReconciler reconciles a IngressTLSLog object
type IngressTLSLogReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Interval records the interval for regeneration of TLS logs
	Interval time.Duration
	// IngressErrorMap stores the error of each ingress
	// If the ingress's error exists, skip
	// If not, add or update the ingress's error
	IngressErrorMap map[string]error

	//IngressUpdateTimeMap records the update time of the ingress
	IngressUpdateTimeMap map[string]time.Time
}

// ingressAuditorNamespace where the project is deployed in
const (
	ingressAuditorNamespace = "ingress-auditor-system"
	ErrLogLevel             = "Error"
)

var ErrFetchIngree = errors.New("unable to fetch ingress")
var ErrSecretNameMissing = errors.New("the secretName does not define in ingress")
var ErrFetchSecret = errors.New("unable to fetch secret")
var ErrCrtOrKeyMissing = errors.New("the crt or key does not exist in secret")
var ErrHostsMissing = errors.New("the Hosts does not define in ingress")
var ErrTLSVerification = errors.New("TLS verification failed")
var ErrHTTPRedirectMissing = errors.New("TLS is not used and redirect is not applied neither")
var ErrCreateTLSLog = errors.New("failed to create new TLS log")

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
	// Init IngressErrorMap
	if r.IngressErrorMap == nil {
		r.IngressErrorMap = make(map[string]error)
	}

	// Fetch the ingress instance
	ingress := &networkingv1.Ingress{}
	ingressNamespacedName := req.NamespacedName.String()
	ingressName := req.NamespacedName.Name
	ingressNs := req.NamespacedName.Namespace
	err := r.Get(ctx, req.NamespacedName, ingress)
	if err != nil {
		exist := r.checkKeyValue(ingressNamespacedName, ErrFetchIngree)
		// If the error has been recorded, pass
		if exist {
			return ctrl.Result{RequeueAfter: r.Interval}, nil
		}

		// If not, update the error
		if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrFetchIngree, ingressNamespacedName); err != nil {
			log.Error(err, "failed to create log instance")
			return ctrl.Result{RequeueAfter: r.Interval}, nil
		}

		log.Error(err, "unable to fetch ingress")
		return ctrl.Result{RequeueAfter: r.Interval}, nil
	}

	// Check if TLS exists
	// If yes
	if len(ingress.Spec.TLS) != 0 {
		// Check if TLS secret exists
		for _, tls := range ingress.Spec.TLS {
			// Get the secretName
			if tls.SecretName == "" {
				exist := r.checkKeyValue(ingressNamespacedName, ErrSecretNameMissing)
				// If the error has been recorded, pass
				if exist {
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				// If not, update the error
				if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrSecretNameMissing, ingressNamespacedName); err != nil {
					log.Error(err, "failed to create log instance")
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				log.Info("The secretName does not define in ingress")
				// generate a log, "namespace-ingress-TLS secretName is empty."
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			// Fetch the secret
			secret := &v1.Secret{}
			err = r.Get(ctx, types.NamespacedName{Name: tls.SecretName, Namespace: ingress.Namespace}, secret)
			if err != nil {
				exist := r.checkKeyValue(ingressNamespacedName, ErrFetchSecret)
				// If the error has been recorded, pass
				if exist {
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				// If not, update the error
				if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrFetchSecret, ingressNamespacedName); err != nil {
					log.Error(err, "failed to create log instance")
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				log.Error(err, "unable to fetch secret "+tls.SecretName)
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			// Get crt and key
			crt := secret.Data["tls.crt"]
			key := secret.Data["tls.key"]

			if crt == nil || key == nil {
				exist := r.checkKeyValue(ingressNamespacedName, ErrCrtOrKeyMissing)
				// If the error has been recorded, pass
				if exist {
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				// If not, update the error
				if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrCrtOrKeyMissing, ingressNamespacedName); err != nil {
					log.Error(err, "failed to create log instance")
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				log.Info("The crt or key does not exist in secret")
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			// Get the hosts
			if len(tls.Hosts) == 0 {
				exist := r.checkKeyValue(ingressNamespacedName, ErrHostsMissing)
				// If the error has been recorded, pass
				if exist {
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				// If not, update the error
				if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrHostsMissing, ingressNamespacedName); err != nil {
					log.Error(err, "failed to create log instance")
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}

				log.Info("The Hosts does not define in ingress")
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			// For all the hosts, use openssl verifies it
			for _, host := range tls.Hosts {
				err = checkTLS(crt, key, host)
				if err != nil {
					exist := r.checkKeyValue(ingressNamespacedName, ErrTLSVerification)
					// If the error has been recorded, pass
					if exist {
						return ctrl.Result{RequeueAfter: r.Interval}, nil
					}

					// If not, update the error
					if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrTLSVerification, ingressNamespacedName); err != nil {
						log.Error(err, "failed to create log instance")
						return ctrl.Result{RequeueAfter: r.Interval}, nil
					}

					log.Error(err, "TLS verification failed")
					return ctrl.Result{RequeueAfter: r.Interval}, nil
				}
			}

			log.Info(fmt.Sprintf("Ingress %s TLS ia applied correctly", ingressNamespacedName))
		}
	} else {
		// If not, check if redirect exist.
		if len(ingress.Annotations) == 0 {
			exist := r.checkKeyValue(ingressNamespacedName, ErrHTTPRedirectMissing)
			// If the error has been recorded, pass
			if exist {
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			// If not, update the error
			if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrHTTPRedirectMissing, ingressNamespacedName); err != nil {
				log.Error(err, "failed to create log instance")
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			log.Info("TLS is not used and redirect is not applied neither")
			return ctrl.Result{RequeueAfter: r.Interval}, nil
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
			exist := r.checkKeyValue(ingressNamespacedName, ErrHTTPRedirectMissing)
			// If the error has been recorded, pass
			if exist {
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			// If not, update the error
			if err := r.logErrorAndUpdateMaps(ctx, ingressNs, ingressName, ErrHTTPRedirectMissing, ingressNamespacedName); err != nil {
				log.Error(err, "failed to create log instance")
				return ctrl.Result{RequeueAfter: r.Interval}, nil
			}

			log.Info("TLS is not used and redirect is not applied neither")
			return ctrl.Result{RequeueAfter: r.Interval}, nil
		}

		log.Info(fmt.Sprintf("Ingress %s TLS is not used but redirect is applied", ingressNamespacedName))
	}
	// If does not exist, check if not exist in the map, generate one error crd for that ingress
	// Add it to map
	// If exists, continue

	return ctrl.Result{RequeueAfter: r.Interval}, nil
}

// checkKeyValue checks if the given key exists in the map and has the specified value.
func (r *IngressTLSLogReconciler) checkKeyValue(key string, value error) bool {
	// Only the key exists, value is the same and updateTime within lastUpdatTime add with interval, returns true
	if v, ok := r.IngressErrorMap[key]; ok && v == value {
		if lastUpdateTime, ok := r.IngressUpdateTimeMap[key]; ok {
			if time.Now().Before(lastUpdateTime.Add(r.Interval)) {
				return true
			}
		}
	}

	return false
}

// logErrorAndUpdateMaps updates maps of IngressUpdateTimeMap and IngressErrorMap
func (r *IngressTLSLogReconciler) updateValueForKey(key string, value error, updateTime time.Time) time.Time {
	r.IngressUpdateTimeMap[key] = updateTime
	r.IngressErrorMap[key] = value
	return updateTime
}

// createTLSLog creates ingresstlslogs instance
func (r *IngressTLSLogReconciler) createTLSLog(ingressNamespace string, ingressName string, err error, updateTime time.Time) *ingressauditv1alpha1.IngressTLSLog {
	timeStr := updateTime.Format("2006-01-02-15-04-05")
	uniqueSuffix := fmt.Sprintf("%04d", rand.Intn(10000)) // random 4-digit number
	log := &ingressauditv1alpha1.IngressTLSLog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s-%s", ingressNamespace, ingressName, timeStr, uniqueSuffix),
			Namespace: ingressAuditorNamespace,
		},
		Spec: ingressauditv1alpha1.IngressTLSLogSpec{
			LogLevel:            ErrLogLevel,
			NameSpace:           ingressNamespace,
			IngressName:         ingressName,
			Message:             err.Error(),
			GenerationTimestamp: &metav1.Time{Time: updateTime},
		},
	}

	return log
}

// logErrorAndUpdateMaps creates ingresstlslogs instance and updates maps of IngressUpdateTimeMap and IngressErrorMap
func (r *IngressTLSLogReconciler) logErrorAndUpdateMaps(ctx context.Context, ingressNs, ingressName string, errType error, ingressNamespacedName string) error {
	updateTime := time.Now()
	TLSlog := r.createTLSLog(ingressNs, ingressName, errType, updateTime)
	if err := r.Create(ctx, TLSlog); err != nil {
		return err
	}

	r.updateValueForKey(ingressNamespacedName, errType, updateTime)

	return nil
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
