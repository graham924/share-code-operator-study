package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// App is the Schema for the apps API
type App struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppSpec   `json:"spec,omitempty"`
	Status AppStatus `json:"status,omitempty"`
}

// AppSpec defines the desired state of App
type AppSpec struct {
	DeploymentSpec DeploymentTemplate `json:"deploymentTemplate,omitempty"`
	ServiceSpec    ServiceTemplate    `json:"serviceTemplate,omitempty"`
}

type DeploymentTemplate struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Replicas int32  `json:"replicas"`
}

type ServiceTemplate struct {
	Name string `json:"name"`
}

// AppStatus defines the observed state of App.
// It should always be reconstructable from the state of the cluster and/or outside world.
type AppStatus struct {
	//DeploymentStatus *appsv1.DeploymentStatus `json:"deploymentStatus,omitempty"`
	//ServiceStatus    *corev1.ServiceStatus    `json:"serviceStatus,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AppList contains a list of App
type AppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []App `json:"items"`
}
