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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IngressTLSLogSpec defines the desired state of IngressTLSLog
type IngressTLSLogSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// +kubebuilder:validation:Enum=Error;Warn;Info
	// LogLevel defines the severity of the log, including error, warn, info logs.
	// +required
	LogLevel string `json:"level"`

	// +kubebuilder:validation:MaxLength=15
	// +kubebuilder:validation:MinLength=1
	// +required
	NameSpace string `json:"namespace"`

	// +kubebuilder:validation:MaxLength=25
	// +kubebuilder:validation:MinLength=1
	// +required
	IngressName string `json:"ingressName"`

	// +kubebuilder:validation:MaxLength=120
	// +kubebuilder:validation:MinLength=1
	// +required
	Message string `json:"message"`

	// Timestamp records the generation timestamp of the log for interval control.
	// +required
	GenerationTimestamp *metav1.Time `json:"generationTimestamp"`
}

// IngressTLSLogStatus defines the observed state of IngressTLSLog.
type IngressTLSLogStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the IngressTLSLog resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// IngressTLSLog is the Schema for the ingresstlslogs API
type IngressTLSLog struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of IngressTLSLog
	// +required
	Spec IngressTLSLogSpec `json:"spec"`

	// status defines the observed state of IngressTLSLog
	// +optional
	Status IngressTLSLogStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// IngressTLSLogList contains a list of IngressTLSLog
type IngressTLSLogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []IngressTLSLog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IngressTLSLog{}, &IngressTLSLogList{})
}
