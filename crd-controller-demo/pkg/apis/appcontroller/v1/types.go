package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	DeploymentTemplate appsv1.Deployment `json:"deploymentTemplate,omitempty"`
	ServiceTemplate    corev1.Service    `json:"serviceTemplate,omitempty"`
}

// AppStatus defines the observed state of App.
// It should always be reconstructable from the state of the cluster and/or outside world.
type AppStatus struct {
	DeploymentStatus appsv1.DeploymentStatus `json:"deploymentStatus,omitempty"`
	ServiceStatus    corev1.ServiceStatus    `json:"serviceStatus,omitempty"`
}

//type DeployTemplate struct {
//	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
//	Spec              appsv1.DeploymentSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
//}
//
//type SvcTemplate struct {
//	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
//	Spec              corev1.ServiceSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
//}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AppList contains a list of App
type AppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []App `json:"items"`
}
