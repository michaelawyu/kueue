/*
Copyright (c) Microsoft Corporation.
Licensed under the MIT license.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// LabelRBACSetupManagedBy is the label key that indicates the owner of a RBAC setup.
	// The value should be the name of a multi-cluster management platform.
	LabelRBACSetupManagedBy = "x-k8s.io/rbac-setup-managed-by"
)

const (
	// Condition types for AuthTokenRequest status.
	SvcAccountFoundOrCreatedCondType  = "ServiceAccountFoundOrCreated"
	SvcAccountTokenRetrievedCondType  = "ServiceAccountTokenRetrieved"
	RoleFoundOrCreatedCondType        = "RoleFoundOrCreated"
	ClusterRoleFoundOrCreatedCondType = "ClusterRoleFoundOrCreated"
	RoleBindingCreatedCondType        = "RoleBindingCreated"
	ClusterRoleBindingCreatedCondType = "ClusterRoleBindingCreated"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories={cluster-inventory},shortName=atr
// +kubebuilder:subresource:status
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AuthTokenRequest represents a request for an authentication token.
type AuthTokenRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the AuthTokenRequest.
	// +required
	Spec AuthTokenRequestSpec `json:"spec"`

	// Status is the observed state of the AuthTokenRequest.
	// +optional
	Status AuthTokenRequestStatus `json:"status,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,categories={cluster-inventory},shortName=atr
// +kubebuilder:subresource:status
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InternalAuthTokenRequest is the internal representation of an AuthTokenRequest.
type InternalAuthTokenRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the InternalAuthTokenRequest.
	// +required
	Spec AuthTokenRequestSpec `json:"spec"`

	// Status is the observed state of the InternalAuthTokenRequest.
	// +optional
	Status AuthTokenRequestStatus `json:"status,omitempty"`
}

// AuthTokenRequestSpec defines the desired state of AuthTokenRequest.
type AuthTokenRequestSpec struct {
	// TargetClusterProfile is the cluster profile to which the authentication token is requested.
	//
	// Note that the API group and kind of the object reference must be left empty.
	//
	// This field is immutable after creation.
	//
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="TargetClusterProfile is immutable"
	TargetClusterProfile corev1.TypedObjectReference `json:"targetClusterProfile"`

	// ServiceAccountName is the name of the service account on behalf of which the authentication
	// token is issued.
	//
	// If the service account cannot be found in the cluster, the system will attempt to create it.
	// Otherwise, the system will request and retrieve a token for the service account directly,
	// even if the roles and/or cluster roles are not set up as they are expected in the request.
	//
	// This field is immutable after creation.
	//
	// For simplicity reasons, for now the API assumes that the token issuer (the multi-cluster
	// management platform) decides where the service account lives.
	//
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ServiceAccountName is immutable"
	// +kubebuilder:validation:MaxLength=63
	ServiceAccountName string `json:"serviceAccountName"`

	// ServiceAccountNamespace is the namespace where the service account is created.
	//
	// If left empty, it is up to the token issuer to decide where the service account lives.
	//
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ServiceAccountNamespace is immutable"
	// +kubebuilder:validation:MaxLength=63
	ServiceAccountNamespace string `json:"serviceAccountNamespace"`

	// Roles is a list of roles that should be bound to the service account.
	//
	// If a role in the list is not found, the system will attempt to create it;
	// otherwise, the system will bind the role directly to the service account, even if the policies
	// specified in the role object do not match with those in the request.
	//
	// This field is immutable after creation.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Roles is immutable"
	Roles []Role `json:"roles"`

	// ClusterRoles is a list of cluster roles that should be bound to the service account.
	//
	// If a cluster role in the list is not found, the system will attempt to create it;
	// otherwise, the system will bind the cluster role directly to the service account,
	// even if the policies specified in the cluster role object do not match with those
	// in the request.
	//
	// This field is immutable after creation.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ClusterRoles is immutable"
	ClusterRoles []ClusterRole `json:"clusterRoles"`
}

// Role defines a role that should be bound to the service account.
type Role struct {
	// Namespace is the namespace where the role is created.
	// The namespace will be created if it does not already exist.
	// +required
	Namespace string `json:"namespace"`

	// Name is the name of the role that should be created.
	// +required
	Name string `json:"name"`

	// Rules is a list of policies for the resources in the specified namespace.
	// +optional
	// +listType=atomic
	Rules []rbacv1.PolicyRule `json:"rules"`
}

// ClusterRole describes a set of permissions that should be set under the cluster scope.
type ClusterRole struct {
	// Name is the name of the cluster role that should be created.
	// +required
	Name string `json:"name"`

	// Rules is a list of policies for the resources in the cluster scope.
	// +optional
	// +listType=atomic
	Rules []rbacv1.PolicyRule `json:"rules"`
}

// AuthTokenRequestStatus defines the observed state of AuthTokenRequest.
type AuthTokenRequestStatus struct {
	// TokenResponse is the issued authentication token.
	// +optional
	TokenResponse corev1.ObjectReference `json:"tokenResponse,omitempty"`

	// Conditions is an array of conditions for the authentication token request.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true

// AuthTokenRequestList contains a list of AuthTokenRequests.
type AuthTokenRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthTokenRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthTokenRequest{}, &AuthTokenRequestList{})
}
