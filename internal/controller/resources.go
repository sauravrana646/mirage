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
	"errors"
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

const (
	networkPolicyBaselineName     = "mirage-baseline"
	networkPolicyDependenciesName = "mirage-dependencies"
	previewComponent              = "preview"
)

func (r *PreviewEnvironmentReconciler) ensureChildren(ctx context.Context, resolved *resolvedPreview) error {
	pe := resolved.PE
	if err := r.ensureLimitRange(ctx, pe); err != nil {
		return err
	}
	if err := r.ensureResourceQuota(ctx, pe); err != nil {
		return err
	}
	if err := r.ensureNetworkPolicy(ctx, resolved); err != nil {
		return err
	}
	if err := r.ensureDependencies(ctx, pe); err != nil {
		return err
	}
	desiredNames := map[string]struct{}{}
	for _, svc := range resolved.Services {
		desiredNames[svc.WorkloadName] = struct{}{}
		if _, err := r.ensureServiceDeployment(ctx, resolved, svc); err != nil {
			return err
		}
		if err := r.ensureServiceService(ctx, pe, svc); err != nil {
			return err
		}
		if svc.Ingress != nil && svc.Ingress.Enabled {
			if err := r.ensureServiceIngress(ctx, pe, svc); err != nil {
				return err
			}
		} else if err := r.deleteNamedIngress(ctx, pe.Spec.TargetNamespace, svc.WorkloadName); err != nil {
			return err
		}
	}
	return r.pruneStaleWorkloads(ctx, pe, desiredNames)
}

func (r *PreviewEnvironmentReconciler) pruneStaleWorkloads(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, desired map[string]struct{}) error {
	ns := pe.Spec.TargetNamespace
	var deploys appsv1.DeploymentList
	if err := r.List(ctx, &deploys, client.InNamespace(ns), client.MatchingLabels{
		miragev1alpha1.LabelManagedBy: miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:  string(pe.UID),
	}); err != nil {
		return err
	}
	var errs []error
	for i := range deploys.Items {
		d := &deploys.Items[i]
		// Ephemeral dependencies are managed separately (component=dependency / LabelDependency).
		if d.Labels[componentLabel] == depComponent || d.Labels[miragev1alpha1.LabelDependency] != "" {
			continue
		}
		if _, ok := desired[d.Name]; ok {
			continue
		}
		if err := r.deleteIgnoreNotFound(ctx, d); err != nil {
			errs = append(errs, err)
		}
		if err := r.deleteIgnoreNotFound(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: ns}}); err != nil {
			errs = append(errs, err)
		}
		if err := r.deleteIgnoreNotFound(ctx, &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: ns}}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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

func (r *PreviewEnvironmentReconciler) ensureNamespace(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, psa string) error {
	if psa == "" {
		psa = string(miragev1alpha1.SecurityProfileRestricted)
	}
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
					"pod-security.kubernetes.io/enforce": psa,
					"pod-security.kubernetes.io/warn":    psa,
					"pod-security.kubernetes.io/audit":   psa,
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
	// Reconcile PSA labels on existing owned namespaces (create path sets them once).
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}
	psaKeys := []string{
		"pod-security.kubernetes.io/enforce",
		"pod-security.kubernetes.io/warn",
		"pod-security.kubernetes.io/audit",
	}
	changed := false
	for _, k := range psaKeys {
		if ns.Labels[k] != psa {
			ns.Labels[k] = psa
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return r.Update(ctx, ns)
}

func (r *PreviewEnvironmentReconciler) workloadLabels(pe *miragev1alpha1.PreviewEnvironment, serviceName string) map[string]string {
	labels := make(map[string]string, len(pe.Spec.WorkloadLabels)+7)
	for k, v := range pe.Spec.WorkloadLabels {
		if isReservedWorkloadLabel(k) {
			continue
		}
		labels[k] = v
	}
	// Mirage-owned labels must always win.
	labels[appLabel] = pe.Name
	labels[componentLabel] = previewComponent
	labels[miragev1alpha1.LabelManagedBy] = miragev1alpha1.ManagedByValue
	labels[miragev1alpha1.LabelOwnerUID] = string(pe.UID)
	labels[miragev1alpha1.LabelOwnerName] = pe.Name
	labels[miragev1alpha1.LabelOwnerNamespace] = pe.Namespace
	labels[miragev1alpha1.LabelService] = serviceName
	return labels
}

func isReservedWorkloadLabel(key string) bool {
	switch key {
	case appLabel, componentLabel,
		miragev1alpha1.LabelManagedBy,
		miragev1alpha1.LabelOwnerUID,
		miragev1alpha1.LabelOwnerName,
		miragev1alpha1.LabelOwnerNamespace,
		miragev1alpha1.LabelService,
		miragev1alpha1.LabelDependency:
		return true
	default:
		return false
	}
}

func (r *PreviewEnvironmentReconciler) ensureLimitRange(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	lr := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "mirage-defaults", Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, lr, func() error {
		lr.Labels = r.workloadLabels(pe, pe.Name)
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
		rq.Labels = r.workloadLabels(pe, pe.Name)
		rq.Spec.Hard = corev1.ResourceList{
			corev1.ResourceRequestsCPU: resource.MustParse("2"), corev1.ResourceRequestsMemory: resource.MustParse("2Gi"),
			corev1.ResourceLimitsCPU: resource.MustParse("4"), corev1.ResourceLimitsMemory: resource.MustParse("4Gi"),
			corev1.ResourcePods: resource.MustParse("20"),
		}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) ensureNetworkPolicy(ctx context.Context, resolved *resolvedPreview) error {
	pe := resolved.PE
	mode := pe.Spec.NetworkPolicy
	if mode == "" {
		mode = miragev1alpha1.NetworkPolicyBaseline
	}
	if mode == miragev1alpha1.NetworkPolicyDisabled {
		var errs []error
		for _, name := range []string{networkPolicyBaselineName, networkPolicyDependenciesName} {
			np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: pe.Spec.TargetNamespace}}
			if err := r.deleteIgnoreNotFound(ctx, np); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	ports := make([]networkingv1.NetworkPolicyPort, 0, len(resolved.Services))
	seen := map[int32]struct{}{}
	for _, svc := range resolved.Services {
		if _, ok := seen[svc.Port]; ok {
			continue
		}
		seen[svc.Port] = struct{}{}
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(svc.Port),
		})
	}
	if len(ports) == 0 {
		ports = []networkingv1.NetworkPolicyPort{{
			Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(defaultContainerPort),
		}}
	}
	np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: networkPolicyBaselineName, Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = r.workloadLabels(pe, pe.Name)
		np.Spec.PodSelector = metav1.LabelSelector{MatchLabels: map[string]string{appLabel: pe.Name}}
		np.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}
		// Ingress from ingress-controller namespaces and same-preview app pods (frontend→api).
		np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{
			Ports: ports,
			From: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"mirage.dev/ingress-access": "true"},
					},
				},
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							miragev1alpha1.LabelOwnerUID: string(pe.UID),
							componentLabel:               previewComponent,
						},
					},
				},
			},
		}}
		if mode == miragev1alpha1.NetworkPolicyPermissive {
			np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
		} else {
			egress := []networkingv1.NetworkPolicyEgressRule{{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: protocolPtr(corev1.ProtocolUDP), Port: intstrPtr(53)},
					{Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(53)},
				},
			}}
			// Same-preview app↔app (frontend→api): NP requires matching ingress+egress peers.
			egress = append(egress, networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							miragev1alpha1.LabelOwnerUID: string(pe.UID),
							componentLabel:               previewComponent,
						},
					},
				}},
				Ports: ports,
			})
			// Allow preview pods to reach in-namespace dependencies.
			if deps := enabledDependencyKinds(pe.Spec.Dependencies); len(deps) > 0 {
				depPorts := make([]networkingv1.NetworkPolicyPort, 0, len(deps))
				for _, d := range deps {
					depPorts = append(depPorts, networkingv1.NetworkPolicyPort{
						Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(d.Port),
					})
				}
				egress = append(egress, networkingv1.NetworkPolicyEgressRule{
					To: []networkingv1.NetworkPolicyPeer{{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{componentLabel: depComponent},
						},
					}},
					Ports: depPorts,
				})
			}
			np.Spec.Egress = egress
		}
		return nil
	})
	if err != nil {
		return err
	}
	return r.ensureDependencyNetworkPolicy(ctx, pe, mode)
}

// ensureDependencyNetworkPolicy isolates dependency pods: ingress only from preview app
// pods (not other deps), egress DNS-only in baseline mode.
func (r *PreviewEnvironmentReconciler) ensureDependencyNetworkPolicy(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, mode miragev1alpha1.NetworkPolicyMode) error {
	ns := pe.Spec.TargetNamespace
	npMeta := metav1.ObjectMeta{Name: networkPolicyDependenciesName, Namespace: ns}
	if !dependenciesEnabled(pe.Spec.Dependencies) {
		return r.deleteIgnoreNotFound(ctx, &networkingv1.NetworkPolicy{ObjectMeta: npMeta})
	}
	np := &networkingv1.NetworkPolicy{ObjectMeta: npMeta}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = r.workloadLabels(pe, pe.Name)
		np.Spec.PodSelector = metav1.LabelSelector{MatchLabels: map[string]string{componentLabel: depComponent}}
		np.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}
		depPorts := make([]networkingv1.NetworkPolicyPort, 0)
		for _, d := range enabledDependencyKinds(pe.Spec.Dependencies) {
			depPorts = append(depPorts, networkingv1.NetworkPolicyPort{
				Protocol: protocolPtr(corev1.ProtocolTCP), Port: intstrPtr(d.Port),
			})
		}
		np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{
			Ports: depPorts,
			From: []networkingv1.NetworkPolicyPeer{{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						miragev1alpha1.LabelOwnerUID: string(pe.UID),
						componentLabel:               previewComponent,
					},
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

func (r *PreviewEnvironmentReconciler) ensureServiceDeployment(ctx context.Context, resolved *resolvedPreview, svc effectiveService) (*appsv1.Deployment, error) {
	pe := resolved.PE
	labels := r.workloadLabels(pe, svc.Name)
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: svc.WorkloadName, Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Labels = labels
		replicas := svc.Replicas
		deploy.Spec.Replicas = &replicas
		if deploy.Spec.Selector == nil {
			deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{
				appLabel:                    pe.Name,
				miragev1alpha1.LabelService: svc.Name,
			}}
		}
		deploy.Spec.Template.ObjectMeta.Labels = labels
		if pe.Spec.WorkloadAnnotations != nil {
			deploy.Spec.Template.ObjectMeta.Annotations = pe.Spec.WorkloadAnnotations
		}
		automount := false
		container := corev1.Container{
			Name: "app", Image: svc.Image,
			Env:     mergeEnvPreferUser(svc.Env, dependencyEnvVars(pe.Spec.Dependencies)),
			EnvFrom: svc.EnvFrom,
			Command: svc.Command, Args: svc.Args, Resources: svc.Resources,
			Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: svc.Port}},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: boolPtr(false),
				RunAsNonRoot:             boolPtr(true),
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
		}
		if svc.ImagePullPolicy != "" {
			container.ImagePullPolicy = svc.ImagePullPolicy
		}
		if svc.ReadinessProbe != nil {
			container.ReadinessProbe = httpProbe(svc.ReadinessProbe, svc.Port)
		}
		if svc.LivenessProbe != nil {
			container.LivenessProbe = httpProbe(svc.LivenessProbe, svc.Port)
		}
		deploy.Spec.Template.Spec.AutomountServiceAccountToken = &automount
		deploy.Spec.Template.Spec.Containers = []corev1.Container{container}
		deploy.Spec.Template.Spec.ImagePullSecrets = pe.Spec.ImagePullSecrets
		deploy.Spec.Template.Spec.PriorityClassName = pe.Spec.PriorityClassName
		deploy.Spec.Template.Spec.ServiceAccountName = pe.Spec.ServiceAccountName
		deploy.Spec.Template.Spec.NodeSelector = pe.Spec.NodeSelector
		deploy.Spec.Template.Spec.Tolerations = pe.Spec.Tolerations
		deploy.Spec.Template.Spec.Affinity = pe.Spec.Affinity
		if resolved.RuntimeClassName != "" {
			rc := resolved.RuntimeClassName
			deploy.Spec.Template.Spec.RuntimeClassName = &rc
		}
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

func (r *PreviewEnvironmentReconciler) ensureServiceService(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, svc effectiveService) error {
	svcObj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: svc.WorkloadName, Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svcObj, func() error {
		svcObj.Labels = r.workloadLabels(pe, svc.Name)
		svcObj.Spec.Selector = map[string]string{
			appLabel:                    pe.Name,
			miragev1alpha1.LabelService: svc.Name,
		}
		svcObj.Spec.Ports = []corev1.ServicePort{{
			Name: "http", Port: 80, TargetPort: intstr.FromInt32(svc.Port), Protocol: corev1.ProtocolTCP,
		}}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) ensureServiceIngress(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, svc effectiveService) error {
	if svc.Ingress == nil || svc.Ingress.Host == "" {
		return fmt.Errorf("ingress.enabled requires ingress.host for service %q", svc.Name)
	}
	path := svc.Ingress.Path
	if path == "" {
		path = "/"
	}
	pathType := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: svc.WorkloadName, Namespace: pe.Spec.TargetNamespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ing, func() error {
		ing.Labels = r.workloadLabels(pe, svc.Name)
		ing.Annotations = svc.Ingress.Annotations
		if ing.Annotations == nil {
			ing.Annotations = map[string]string{}
		}
		ing.Spec.IngressClassName = svc.Ingress.IngressClassName
		ing.Spec.Rules = []networkingv1.IngressRule{{
			Host: svc.Ingress.Host,
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					Path: path, PathType: &pathType,
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: svc.WorkloadName, Port: networkingv1.ServiceBackendPort{Name: "http"},
						},
					},
				}},
			}},
		}}
		if svc.Ingress.TLS != nil && svc.Ingress.TLS.Enabled {
			secret := svc.Ingress.TLS.SecretName
			if secret == "" {
				secret = svc.WorkloadName + "-tls"
			}
			ing.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{svc.Ingress.Host}, SecretName: secret}}
		} else {
			ing.Spec.TLS = nil
		}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) deleteNamedIngress(ctx context.Context, ns, name string) error {
	return r.deleteIgnoreNotFound(ctx, &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})
}

// cleanupDirectWorkloads removes Deployment/Service/Ingress when switching away from the direct backend.
func (r *PreviewEnvironmentReconciler) cleanupDirectWorkloads(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	ns := pe.Spec.TargetNamespace
	if ns == "" {
		return nil
	}
	var deploys appsv1.DeploymentList
	if err := r.List(ctx, &deploys, client.InNamespace(ns), client.MatchingLabels{
		miragev1alpha1.LabelManagedBy: miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:  string(pe.UID),
	}); err != nil {
		return err
	}
	var errs []error
	for i := range deploys.Items {
		d := &deploys.Items[i]
		if err := r.deleteIgnoreNotFound(ctx, d); err != nil {
			errs = append(errs, err)
		}
		if err := r.deleteIgnoreNotFound(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: ns}}); err != nil {
			errs = append(errs, err)
		}
		if err := r.deleteIgnoreNotFound(ctx, &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: ns}}); err != nil {
			errs = append(errs, err)
		}
		if d.Labels[componentLabel] == depComponent || d.Labels[miragev1alpha1.LabelDependency] != "" {
			if err := r.deleteIgnoreNotFound(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: ns}}); err != nil {
				errs = append(errs, err)
			}
		}
	}
	// Legacy single-name cleanup if labels were not yet present.
	for _, obj := range []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: ns}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: ns}},
		&networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: pe.Name, Namespace: ns}},
	} {
		if err := r.deleteIgnoreNotFound(ctx, obj); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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

func serviceURL(svc effectiveService) string {
	if svc.Ingress == nil || !svc.Ingress.Enabled || svc.Ingress.Host == "" {
		return ""
	}
	scheme := "http"
	if svc.Ingress.TLS != nil && svc.Ingress.TLS.Enabled {
		scheme = "https"
	}
	path := svc.Ingress.Path
	if path == "" || path == "/" {
		return scheme + "://" + svc.Ingress.Host
	}
	if path[0] != '/' {
		path = "/" + path
	}
	return scheme + "://" + svc.Ingress.Host + path
}
