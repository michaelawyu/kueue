/*
Copyright 2024 The Kubernetes Authors.

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

package clusterinventory

import (
	"context"
	"slices"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterinventoryv1alpha1 "sigs.k8s.io/kueue/apis/clusterinventory/v1alpha1"
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

const (
	// authTokenReqeustCleanupFinalizer is the finalizer added to the ClusterProfile object
	// if an AuthTokenRequest object has been created.
	authTokenReqeustCleanupFinalizer = "kueue.x-k8s.io/auth-token-request-cleanup"

	svcAccountName      = "multikueue"
	svcAccountNamespace = "kueue-system"
	clusterRoleName     = "multikueue"
)

// Reconciler reconciles a ClusterProfile object and performs setup so that the
// target cluster can be used in MultiKueue.
type Reconciler struct {
	HubClient           client.Client
	MultiClusterManager string
	KueueNS             string
}

// Reconcile reconciles the ClusterProfile object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cpRef := klog.KRef(req.Namespace, req.Name)
	startTime := time.Now()
	klog.V(2).InfoS("Reconciliation starts (cluster profile controller)", "ClusterProfile", cpRef)
	defer func() {
		latency := time.Since(startTime).Milliseconds()
		klog.V(2).InfoS("Reconciliation ends (cluster profile controller)", "ClusterProfile", cpRef, "Latency", latency)
	}()

	// Retrieve the ClusterProfile object.
	cp := &clusterinventoryv1alpha1.ClusterProfile{}
	if err := r.HubClient.Get(ctx, req.NamespacedName, cp); err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).InfoS("ClusterProfile is not found; skip the reconciliation", "ClusterProfile", cpRef)
			return ctrl.Result{}, nil
		}
		klog.ErrorS(err, "Failed to get ClusterProfile", "ClusterProfile", cpRef)
		return ctrl.Result{}, err
	}

	if cp.Spec.ClusterManager.Name != r.MultiClusterManager {
		// The cluster profile is not managed by the expected platform; ignore it.
		klog.V(4).InfoS("ClusterProfile is not managed by the expected platform; skip the reconciliation", "ClusterProfile", cpRef, "ExpectedClusterManager", r.MultiClusterManager, "ActualClusterManager", cp.Spec.ClusterManager.Name)
		return ctrl.Result{}, nil
	}

	// Check if the ClusterProfile object has been marked for deletion.
	if !cp.DeletionTimestamp.IsZero() {
		klog.V(2).InfoS("ClusterProfile has been marked for deletion; attempt to delete any AuthTokenRequest object sent", "ClusterProfile", cpRef)

		// Check if the AuthTokenRequest object has been created.
		atr := &clusterinventoryv1alpha1.AuthTokenRequest{}
		atrTypedName := client.ObjectKey{Namespace: r.KueueNS, Name: cp.Name}
		err := r.HubClient.Get(ctx, atrTypedName, atr)
		switch {
		case err != nil && !errors.IsNotFound(err):
			// An unexpected error occurred.
			klog.ErrorS(err, "Failed to get AuthTokenRequest", "ClusterProfile", cpRef, "AuthTokenRequest", klog.KObj(atr))
			return ctrl.Result{}, err
		case err == nil:
			// An AuthTokenRequest object has been created; delete it.
			if err := r.HubClient.Delete(ctx, atr); err != nil {
				klog.ErrorS(err, "Failed to delete AuthTokenRequest", "ClusterProfile", cpRef, "AuthTokenRequest", klog.KObj(atr))
				return ctrl.Result{}, err
			}
		default:
			// The AuthTokenRequest object has not been created; do nothing.
		}

		// Remove the finalizer.
		controllerutil.RemoveFinalizer(cp, authTokenReqeustCleanupFinalizer)
		if err := r.HubClient.Update(ctx, cp); err != nil {
			klog.ErrorS(err, "Failed to remove the cleanup finalizer", "ClusterProfile", cpRef)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Add the finalizer.
	if !controllerutil.ContainsFinalizer(cp, authTokenReqeustCleanupFinalizer) {
		controllerutil.AddFinalizer(cp, authTokenReqeustCleanupFinalizer)
		if err := r.HubClient.Update(ctx, cp); err != nil {
			klog.ErrorS(err, "Failed to add the cleanup finalizer", "ClusterProfile", cpRef)
			return ctrl.Result{}, err
		}
	}

	// Check if the AuthTokenRequest object has been created.
	atr := &clusterinventoryv1alpha1.AuthTokenRequest{}
	atrTypedName := client.ObjectKey{Namespace: r.KueueNS, Name: cp.Name}
	err := r.HubClient.Get(ctx, atrTypedName, atr)
	switch {
	case err != nil && !errors.IsNotFound(err):
		// An unexpected error occurred.
		klog.ErrorS(err, "Failed to get AuthTokenRequest", "ClusterProfile", cpRef, "AuthTokenRequest", klog.KObj(atr))
		return ctrl.Result{}, err
	case errors.IsNotFound(err):
		// An AuthTokenRequest object has never been created; create one.
		klog.V(2).InfoS("No AuthTokenRequest created; prepare one", "ClusterProfile", cpRef, "AuthTokenRequest", klog.KObj(atr))
		atr = &clusterinventoryv1alpha1.AuthTokenRequest{
			ObjectMeta: ctrl.ObjectMeta{
				Namespace: r.KueueNS,
				Name:      cp.Name,
			},
			Spec: clusterinventoryv1alpha1.AuthTokenRequestSpec{
				TargetClusterProfile: corev1.TypedObjectReference{
					APIGroup:  &clusterinventoryv1alpha1.GroupVersion.Group,
					Kind:      "ClusterProfile",
					Name:      cp.Name,
					Namespace: &cp.Namespace,
				},
				ServiceAccountName:      svcAccountName,
				ServiceAccountNamespace: svcAccountNamespace,
				ClusterRoles: []clusterinventoryv1alpha1.ClusterRole{
					{
						Name: clusterRoleName,
						Rules: []rbacv1.PolicyRule{
							{
								APIGroups: []string{"batch"},
								Resources: []string{"jobs"},
								Verbs:     []string{"create", "get", "list", "delete", "watch"},
							},
							{
								APIGroups: []string{"batch"},
								Resources: []string{"jobs/status"},
								Verbs:     []string{"get"},
							},
							{
								APIGroups: []string{"jobset.x-k8s.io"},
								Resources: []string{"jobsets/status"},
								Verbs:     []string{"get"},
							},
							{
								APIGroups: []string{"kueue.x-k8s.io"},
								Resources: []string{"workloads"},
								Verbs:     []string{"create", "get", "list", "delete", "watch"},
							},
							{
								APIGroups: []string{"kueue.x-k8s.io"},
								Resources: []string{"workloads/status"},
								Verbs:     []string{"get", "patch", "update"},
							},
						},
					},
				},
			},
		}
		if err := r.HubClient.Create(ctx, atr); err != nil {
			klog.ErrorS(err, "Failed to create AuthTokenRequest", "ClusterProfile", cpRef, "AuthTokenRequest", klog.KObj(atr))
			return ctrl.Result{}, err
		}
		// No need to wait for the AuthTokenRequest object to be reconciled; the controller
		// will be triggered by the status updates on the AuthTokenRequest object.
		return ctrl.Result{}, nil
	default:
		// An AuthTokenRequest object has been created; do the MultiKueue setup.
		klog.V(2).InfoS("An AuthTokenRequest has been created; prepare the MultiKueue setup", "ClusterProfile", cpRef, "AuthTokenRequest", klog.KObj(atr))

		// Retrieve the provided kubeconfig kept in a config map.
		//
		// The system guarantees that the config map is created before the AuthTokenRequest object
		// status is updated.
		cm := &corev1.ConfigMap{}
		cmTypedName := client.ObjectKey{Namespace: r.KueueNS, Name: atr.Name}
		if err := r.HubClient.Get(ctx, cmTypedName, cm); err != nil {
			if errors.IsNotFound(err) {
				klog.V(2).InfoS("ConfigMap is not found; skip the reconciliation",
					"ClusterProfile", cpRef,
					"ConfigMap", klog.KObj(cm),
					"AuthTokenRequest", klog.KObj(atr))
				return ctrl.Result{}, nil
			}
		}

		kubeconfig := cm.Data["kubeconfig"]
		// Do a sanity check.
		if len(kubeconfig) == 0 {
			klog.V(2).InfoS("Extracted kubeconfig is empty; skip the reconciliation",
				"ClusterProfile", cpRef,
				"ConfigMap", klog.KObj(cm),
				"AuthTokenRequest", klog.KObj(atr))
			return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
		}

		// Set up the MultiKueue system.
		secret := &corev1.Secret{
			ObjectMeta: ctrl.ObjectMeta{
				Namespace: r.KueueNS,
				Name:      cp.Name,
			},
		}
		createOrUpdateRes, err := controllerutil.CreateOrUpdate(ctx, r.HubClient, secret, func() error {
			if secret.CreationTimestamp.IsZero() {
				secret.Type = corev1.SecretTypeOpaque
			}
			secret.Data = map[string][]byte{
				"kubeconfig": []byte(kubeconfig),
			}
			return nil
		})
		if err != nil {
			klog.ErrorS(err, "Failed to create or update the secret", "ClusterProfile", cpRef, "Secret", klog.KObj(secret), "Operation", createOrUpdateRes)
			return ctrl.Result{}, err
		}

		mkcluster := &kueuev1alpha1.MultiKueueCluster{
			ObjectMeta: ctrl.ObjectMeta{
				Name: cp.Name,
			},
		}
		createOrUpdateRes, err = controllerutil.CreateOrUpdate(ctx, r.HubClient, mkcluster, func() error {
			mkcluster.Spec.KubeConfig = kueuev1alpha1.KubeConfig{
				Location:     cp.Name,
				LocationType: "Secret",
			}
			return nil
		})
		if err != nil {
			klog.ErrorS(err, "Failed to create or update the MultiKueueCluster", "ClusterProfile", cpRef, "MultiKueueCluster", klog.KObj(mkcluster), "Operation", createOrUpdateRes)
			return ctrl.Result{}, err
		}

		mkconfig := &kueuev1alpha1.MultiKueueConfig{
			ObjectMeta: ctrl.ObjectMeta{
				Name: r.MultiClusterManager,
			},
		}
		createOrUpdateRes, err = controllerutil.CreateOrUpdate(ctx, r.HubClient, mkconfig, func() error {
			if mkconfig.Spec.Clusters == nil {
				mkconfig.Spec.Clusters = []string{}
			}

			if !slices.Contains(mkconfig.Spec.Clusters, cp.Name) {
				cs := append(mkconfig.Spec.Clusters, cp.Name)
				sort.Strings(cs)
				mkconfig.Spec.Clusters = cs
			}

			return nil
		})
		if err != nil {
			klog.ErrorS(err, "Failed to create or update the MultiKueueConfig", "ClusterProfile", cpRef, "MultiKueueConfig", klog.KObj(mkconfig), "Operation", createOrUpdateRes)
			return ctrl.Result{}, err
		}

		ac := &kueuev1beta1.AdmissionCheck{
			ObjectMeta: ctrl.ObjectMeta{
				Name: cp.Name,
			},
		}
		createOrUpdateRes, err = controllerutil.CreateOrUpdate(ctx, r.HubClient, ac, func() error {
			ac.Spec.ControllerName = "kueue.x-k8s.io/multikueue"
			ac.Spec.Parameters = &kueuev1beta1.AdmissionCheckParametersReference{
				APIGroup: "kueue.x-k8s.io",
				Kind:     "MultiKueueConfig",
				Name:     r.MultiClusterManager,
			}

			return nil
		})
		if err != nil {
			klog.ErrorS(err, "Failed to create or update the AdmissionCheck", "ClusterProfile", cpRef, "AdmissionCheck", klog.KObj(ac), "Operation", createOrUpdateRes)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterinventoryv1alpha1.ClusterProfile{}).
		Complete(r)
}
