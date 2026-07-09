package main

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "open-crypto-broker.io", Version: "v1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &CryptoBroker{}, &CryptoBrokerList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type CryptoBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CryptoBrokerSpec   `json:"spec,omitempty"`
	Status CryptoBrokerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:generate=true
type CryptoBrokerSpec struct {
	Profile          string `json:"profile"`
	Environment      string `json:"environment,omitempty"`
	Version          string `json:"version,omitempty"`
	TargetDeployment string `json:"targetDeployment"`
}

// +kubebuilder:object:generate=true
type CryptoBrokerStatus struct {
	State string `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
type CryptoBrokerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []CryptoBroker `json:"items"`
}
