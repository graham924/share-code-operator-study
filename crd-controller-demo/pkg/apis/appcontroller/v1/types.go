package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AppSpec defines the desired state of App
type AppSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS -- desired state of cluster
	DeploymentTemplate appsv1.DeploymentSpec `json:"deploymentTemplate"`
	ServiceTemplate    corev1.ServiceSpec    `json:"serviceTemplate"`
}

// AppStatus defines the observed state of App.
// It should always be reconstructable from the state of the cluster and/or outside world.
type AppStatus struct {
	// INSERT ADDITIONAL STATUS FIELDS -- observed state of cluster
	DeploymentStatus appsv1.DeploymentStatus `json:"deploymentStatus"`
	ServiceStatus    corev1.ServiceStatus    `json:"serviceStatus"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// App is the Schema for the apps API
type App struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppSpec   `json:"spec,omitempty"`
	Status AppStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AppList contains a list of App
type AppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []App `json:"items"`
}
