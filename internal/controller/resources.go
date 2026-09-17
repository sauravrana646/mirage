/*
Copyright 2026 Saurav Rana.

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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func (r *PreviewEnvironmentReconciler) ensureChildren(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	if err := r.ensureLimitRange(ctx, pe); err != nil {
		return err
	}
	if err := r.ensureResourceQuota(ctx, pe); err != nil {
		return err
	}
	if err := r.ensureNetworkPolicy(ctx, pe); err != nil {
		return err
	}
	if _, err := r.ensureDeployment(ctx, pe); err != nil {
		return err
	}
	if err := r.ensureService(ctx, pe); err != nil {
		return err
	}
	if pe.Spec.Ingress != nil && pe.Spec.Ingress.Enabled {
		return r.ensureIngress(ctx, pe)
	}
	return r.deleteIngressIfPresent(ctx, pe)
}

func (r *PreviewEnvironmentReconciler) cleanupTarget(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	if pe.Spec.TargetNamespace == "" {
		return nil
	}
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: pe.Spec.TargetNamespace}, ns)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !ownsNamespace(pe, ns) {
		return nil
	}
	if err := r.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

type namespaceConflictError struct{ name string }

func (e *namespaceConflictError) Error() string {
	return fmt.Sprintf("namespace %q exists but is not owned by this PreviewEnvironment", e.name)
}

func isNamespaceConflict(err error) bool {
	_, ok := err.(*namespaceConflictError)
	return ok
}

func ownsNamespace(pe *miragev1alpha1.PreviewEnvironment, ns *corev1.Namespace) bool {
	if ns.Labels == nil {
		return false
	}
	return ns.Labels[miragev1alpha1.LabelOwnerUID] == string(pe.UID)
}

func (r *PreviewEnvironmentReconciler) ensureNamespace(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: pe.Spec.TargetNamespace}, ns)
	if apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: pe.Spec.TargetNamespace,
				Labels: map[string]string{
					miragev1alpha1.LabelManagedBy:        miragev1alpha1.ManagedByValue,
					miragev1alpha1.LabelOwnerUID:         string(pe.UID),
					miragev1alpha1.LabelOwnerName:        pe.Name,
					miragev1alpha1.LabelOwnerNamespace:   pe.Namespace,
					"pod-security.kubernetes.io/enforce": "restricted",
					"pod-security.kubernetes.io/warn":    "restricted",
					"pod-security.kubernetes.io/audit":   "restricted",
				},
			},
		}
		return r.Create(ctx, ns)
	}
	if err != nil {
		return err
	}
	if !ownsNamespace(pe, ns) {
		return &namespaceConflictError{name: ns.Name}
	}
	return nil
}

func (r *PreviewEnvironmentReconciler) workloadLabels(pe *miragev1alpha1.PreviewEnvironment) map[string]string {
	labels := map[string]string{
		appLabel:                           pe.Name,
		componentLabel:                     "preview",
		miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:       string(pe.UID),
		miragev1alpha1.LabelOwnerName:      pe.Name,
		miragev1alpha1.LabelOwnerNamespace: pe.Namespace,
	}
	for k, v := range pe.Spec.WorkloadLabels {
		labels[k] = v
	}
	return labels
}

func (r *PreviewEnvironmentReconciler) ensureLimitRange(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	lr := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "mirage-defaults", Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, lr, func() error {
		lr.Labels = r.workloadLabels(pe)
		lr.Spec.Limits = []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			Default: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			DefaultRequest: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Max: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) ensureResourceQuota(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	rq := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "mirage-quota", Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rq, func() error {
		rq.Labels = r.workloadLabels(pe)
		rq.Spec.Hard = corev1.ResourceList{
			corev1.ResourceRequestsCPU: resource.MustParse("2"), corev1.ResourceRequestsMemory: resource.MustParse("2Gi"),
			corev1.ResourceLimitsCPU: resource.MustParse("4"), corev1.ResourceLimitsMemory: resource.MustParse("4Gi"),
			corev1.ResourcePods: resource.MustParse("20"),
		}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) ensureNetworkPolicy(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	mode := pe.Spec.NetworkPolicy
	if mode == "" {
		mode = miragev1alpha1.NetworkPolicyBaseline
	}
	if mode == miragev1alpha1.NetworkPolicyDisabled {
		np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "mirage-baseline", Namespace: pe.Spec.TargetNamespace}}
		_ = r.Delete(ctx, np)
		return nil
	}
	port := pe.Spec.ContainerPort
	if port == 0 {
		port = defaultContainerPort
	}
	np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "mirage-baseline", Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = r.workloadLabels(pe)
		np.Spec.PodSelector = metav1.LabelSelector{MatchLabels: map[string]string{appLabel: pe.Name}}
		np.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}
		np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(port)}},
			From: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"mirage.dev/ingress-access": "true"},
				},
			}},
		}}
		if mode == miragev1alpha1.NetworkPolicyPermissive {
			np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
		} else {
			np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: protocolPtr(corev1.ProtocolUDP), Port: intstrPtr(53)},
					{Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(53)},
				},
			}}
		}
		return nil
	})
	return err
}

func protocolPtr(p corev1.Protocol) *corev1.Protocol { return &p }
func intstrPtr(port int32) *intstr.IntOrString {
	v := intstr.FromInt32(port)
	return &v
}
func boolPtr(v bool) *bool    { return &v }
func int32Ptr(v int32) *int32 { return &v }

func (r *PreviewEnvironmentReconciler) ensureDeployment(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (*appsv1.Deployment, error) {
	replicas := defaultReplicas
	if pe.Spec.Replicas != nil {
		replicas = *pe.Spec.Replicas
	}
	port := pe.Spec.ContainerPort
	if port == 0 {
		port = defaultContainerPort
	}
	labels := r.workloadLabels(pe)
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Labels = labels
		deploy.Spec.Replicas = &replicas
		if deploy.Spec.Selector == nil {
			deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{appLabel: pe.Name}}
		}
		deploy.Spec.Template.ObjectMeta.Labels = labels
		if pe.Spec.WorkloadAnnotations != nil {
			deploy.Spec.Template.ObjectMeta.Annotations = pe.Spec.WorkloadAnnotations
		}
		automount := false
		container := corev1.Container{
			Name: "app", Image: pe.Spec.Image, Env: pe.Spec.Env, EnvFrom: pe.Spec.EnvFrom,
			Command: pe.Spec.Command, Args: pe.Spec.Args, Resources: pe.Spec.Resources,
			Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: port}},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: boolPtr(false),
				RunAsNonRoot:             boolPtr(true),
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
		}
		if pe.Spec.ImagePullPolicy != "" {
			container.ImagePullPolicy = pe.Spec.ImagePullPolicy
		}
		if pe.Spec.ReadinessProbe != nil {
			container.ReadinessProbe = httpProbe(pe.Spec.ReadinessProbe, port)
		}
		if pe.Spec.LivenessProbe != nil {
			container.LivenessProbe = httpProbe(pe.Spec.LivenessProbe, port)
		}
		deploy.Spec.Template.Spec.AutomountServiceAccountToken = &automount
		deploy.Spec.Template.Spec.Containers = []corev1.Container{container}
		deploy.Spec.Template.Spec.ImagePullSecrets = pe.Spec.ImagePullSecrets
		deploy.Spec.Template.Spec.PriorityClassName = pe.Spec.PriorityClassName
		deploy.Spec.Template.Spec.ServiceAccountName = pe.Spec.ServiceAccountName
		deploy.Spec.Template.Spec.NodeSelector = pe.Spec.NodeSelector
		deploy.Spec.Template.Spec.Tolerations = pe.Spec.Tolerations
		deploy.Spec.Template.Spec.Affinity = pe.Spec.Affinity
		if pe.Spec.TerminationGracePeriodSeconds != nil {
			deploy.Spec.Template.Spec.TerminationGracePeriodSeconds = pe.Spec.TerminationGracePeriodSeconds
		}
		deploy.Spec.RevisionHistoryLimit = int32Ptr(3)
		deploy.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsNonRoot:   boolPtr(true),
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := r.Get(ctx, types.NamespacedName{Name: deploy.Name, Namespace: deploy.Namespace}, deploy); err != nil {
		return nil, err
	}
	return deploy, nil
}

func httpProbe(p *miragev1alpha1.ProbeSpec, defaultPort int32) *corev1.Probe {
	path := p.Path
	if path == "" {
		path = "/"
	}
	port := p.Port
	if port == 0 {
		port = defaultPort
	}
	delay := p.InitialDelaySeconds
	if delay == 0 {
		delay = 10
	}
	period := p.PeriodSeconds
	if period == 0 {
		period = 10
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: path, Port: intstr.FromInt32(port),
		}},
		InitialDelaySeconds: delay,
		PeriodSeconds:       period,
	}
}

func (r *PreviewEnvironmentReconciler) ensureService(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	port := pe.Spec.ContainerPort
	if port == 0 {
		port = defaultContainerPort
	}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = r.workloadLabels(pe)
		svc.Spec.Selector = map[string]string{appLabel: pe.Name}
		svc.Spec.Ports = []corev1.ServicePort{{
			Name: "http", Port: 80, TargetPort: intstr.FromInt32(port), Protocol: corev1.ProtocolTCP,
		}}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) ensureIngress(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	if pe.Spec.Ingress == nil || pe.Spec.Ingress.Host == "" {
		return fmt.Errorf("ingress.enabled requires ingress.host")
	}
	path := pe.Spec.Ingress.Path
	if path == "" {
		path = "/"
	}
	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ing, func() error {
		ing.Labels = r.workloadLabels(pe)
		ing.Annotations = pe.Spec.Ingress.Annotations
		if ing.Annotations == nil {
			ing.Annotations = map[string]string{}
		}
		ing.Spec.IngressClassName = pe.Spec.Ingress.IngressClassName
		ing.Spec.Rules = []networkingv1.IngressRule{{
			Host: pe.Spec.Ingress.Host,
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					Path: path, PathType: &pathType,
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{Name: pe.Name, Port: networkingv1.ServiceBackendPort{Name: "http"}},
					},
				}},
			}},
		}}
		if pe.Spec.Ingress.TLS != nil && pe.Spec.Ingress.TLS.Enabled {
			secret := pe.Spec.Ingress.TLS.SecretName
			if secret == "" {
				secret = pe.Name + "-tls"
			}
			ing.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{pe.Spec.Ingress.Host}, SecretName: secret}}
		} else {
			ing.Spec.TLS = nil
		}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) deleteIngressIfPresent(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	ing := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: pe.Spec.TargetNamespace}}
	if err := r.Delete(ctx, ing); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// cleanupDirectWorkloads removes Deployment/Service/Ingress when switching away from the direct backend.
func (r *PreviewEnvironmentReconciler) cleanupDirectWorkloads(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	ns := pe.Spec.TargetNamespace
	if ns == "" {
		return nil
	}
	for _, obj := range []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: ns}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: ns}},
		&networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: ns}},
	} {
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *PreviewEnvironmentReconciler) imagePullMessage(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) string {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(pe.Spec.TargetNamespace), client.MatchingLabels{appLabel: pe.Name}); err != nil {
		return ""
	}
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				switch cs.State.Waiting.Reason {
				case "ImagePullBackOff", "ErrImagePull", "InvalidImageName":
					return fmt.Sprintf("%s: %s", cs.State.Waiting.Reason, cs.State.Waiting.Message)
				}
			}
		}
	}
	return ""
}
