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
	"crypto/rand"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	depComponent               = "dependency"
	depNamePrefix              = "mirage-dep-"
	depPostgresKind            = "postgres"
	depRedisKind               = "redis"
	depKafkaKind               = "kafka"
	depPostgresName            = depNamePrefix + depPostgresKind
	depRedisName               = depNamePrefix + depRedisKind
	depKafkaName               = depNamePrefix + depKafkaKind
	depPostgresPort      int32 = 5432
	depRedisPort         int32 = 6379
	depKafkaPort         int32 = 9092
	depPostgresUser            = "preview"
	depPostgresDB              = "preview"
	depSecretKeyUser           = "username"
	depSecretKeyPass           = "password"
	depSecretKeyDB             = "database"
	depSecretKeyDBURL          = "database-url"
	depSecretKeyRedisURL       = "redis-url"
	defaultPostgresVer         = "16"
	defaultRedisVer            = "7.2"
	defaultKafkaVer            = "3.7"
	bitnamiPostgresImage       = "docker.io/bitnami/postgresql"
	bitnamiRedisImage          = "docker.io/bitnami/redis"
	bitnamiKafkaImage          = "docker.io/bitnami/kafka"
	depPasswordBytes           = 32
)

type dependencyKind struct {
	Kind string
	Name string
	Port int32
}

func dependenciesEnabled(deps *miragev1alpha1.PreviewDependenciesSpec) bool {
	if deps == nil {
		return false
	}
	if deps.Postgres != nil && deps.Postgres.Enabled {
		return true
	}
	if deps.Redis != nil && deps.Redis.Enabled {
		return true
	}
	if deps.Kafka != nil && deps.Kafka.Enabled {
		return true
	}
	return false
}

func enabledDependencyKinds(deps *miragev1alpha1.PreviewDependenciesSpec) []dependencyKind {
	if deps == nil {
		return nil
	}
	var out []dependencyKind
	if deps.Postgres != nil && deps.Postgres.Enabled {
		out = append(out, dependencyKind{Kind: depPostgresKind, Name: depPostgresName, Port: depPostgresPort})
	}
	if deps.Redis != nil && deps.Redis.Enabled {
		out = append(out, dependencyKind{Kind: depRedisKind, Name: depRedisName, Port: depRedisPort})
	}
	if deps.Kafka != nil && deps.Kafka.Enabled {
		out = append(out, dependencyKind{Kind: depKafkaKind, Name: depKafkaName, Port: depKafkaPort})
	}
	return out
}

// dependencyEnvVars returns env injected into preview app containers (not dependency pods).
// Credentials are referenced via secretKeyRef so plaintext passwords are not embedded in
// Deployment specs.
func dependencyEnvVars(deps *miragev1alpha1.PreviewDependenciesSpec) []corev1.EnvVar {
	if deps == nil {
		return nil
	}
	var out []corev1.EnvVar
	if deps.Postgres != nil && deps.Postgres.Enabled {
		out = append(out, corev1.EnvVar{
			Name: "DATABASE_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: depPostgresName},
					Key:                  depSecretKeyDBURL,
				},
			},
		})
	}
	if deps.Redis != nil && deps.Redis.Enabled {
		out = append(out, corev1.EnvVar{
			Name: "REDIS_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: depRedisName},
					Key:                  depSecretKeyRedisURL,
				},
			},
		})
	}
	if deps.Kafka != nil && deps.Kafka.Enabled {
		out = append(out, corev1.EnvVar{
			Name:  "KAFKA_BROKERS",
			Value: fmt.Sprintf("%s:%d", depKafkaName, depKafkaPort),
		})
	}
	return out
}

// mergeEnvPreferUser appends injected vars only when the name is not already set by the user.
func mergeEnvPreferUser(user, injected []corev1.EnvVar) []corev1.EnvVar {
	if len(injected) == 0 {
		return user
	}
	seen := make(map[string]struct{}, len(user))
	for _, e := range user {
		seen[e.Name] = struct{}{}
	}
	out := append([]corev1.EnvVar{}, user...)
	for _, e := range injected {
		if _, ok := seen[e.Name]; ok {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (r *PreviewEnvironmentReconciler) dependencyLabels(pe *miragev1alpha1.PreviewEnvironment, kind, resourceName string) map[string]string {
	return map[string]string{
		appLabel:                           resourceName,
		componentLabel:                     depComponent,
		miragev1alpha1.LabelManagedBy:      miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:       string(pe.UID),
		miragev1alpha1.LabelOwnerName:      pe.Name,
		miragev1alpha1.LabelOwnerNamespace: pe.Namespace,
		miragev1alpha1.LabelDependency:     kind,
	}
}

func (r *PreviewEnvironmentReconciler) ensureDependencies(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	deps := pe.Spec.Dependencies
	desired := map[string]struct{}{}
	if deps != nil {
		if deps.Postgres != nil && deps.Postgres.Enabled {
			desired[depPostgresKind] = struct{}{}
			if err := r.ensurePostgresDependency(ctx, pe, deps.Postgres); err != nil {
				return err
			}
		}
		if deps.Redis != nil && deps.Redis.Enabled {
			desired[depRedisKind] = struct{}{}
			if err := r.ensureRedisDependency(ctx, pe, deps.Redis); err != nil {
				return err
			}
		}
		if deps.Kafka != nil && deps.Kafka.Enabled {
			desired[depKafkaKind] = struct{}{}
			if err := r.ensureKafkaDependency(ctx, pe, deps.Kafka); err != nil {
				return err
			}
		}
	}
	return r.pruneDisabledDependencies(ctx, pe, desired)
}

func (r *PreviewEnvironmentReconciler) pruneDisabledDependencies(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, desired map[string]struct{}) error {
	ns := pe.Spec.TargetNamespace
	var deploys appsv1.DeploymentList
	if err := r.List(ctx, &deploys, client.InNamespace(ns), client.MatchingLabels{
		miragev1alpha1.LabelManagedBy: miragev1alpha1.ManagedByValue,
		miragev1alpha1.LabelOwnerUID:  string(pe.UID),
		componentLabel:                depComponent,
	}); err != nil {
		return err
	}
	var errs []error
	for i := range deploys.Items {
		d := &deploys.Items[i]
		depKind := d.Labels[miragev1alpha1.LabelDependency]
		if depKind == "" {
			depKind = d.Name
		}
		if _, ok := desired[depKind]; ok {
			continue
		}
		if err := r.deleteIgnoreNotFound(ctx, d); err != nil {
			errs = append(errs, err)
		}
		if err := r.deleteIgnoreNotFound(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: ns}}); err != nil {
			errs = append(errs, err)
		}
		if err := r.deleteIgnoreNotFound(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: ns}}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func generateDependencyPassword() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, depPasswordBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

func (r *PreviewEnvironmentReconciler) ensurePostgresCredentials(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	ns := pe.Spec.TargetNamespace
	labels := r.dependencyLabels(pe, depPostgresKind, depPostgresName)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: depPostgresName, Namespace: ns}, existing)
	if err == nil {
		// Preserve generated credentials across reconciles; refresh labels only.
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
			existing.Labels = labels
			return nil
		})
		return err
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	pass, err := generateDependencyPassword()
	if err != nil {
		return fmt.Errorf("generate postgres password: %w", err)
	}
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		depPostgresUser, pass, depPostgresName, depPostgresPort, depPostgresDB)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      depPostgresName,
			Namespace: ns,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			depSecretKeyUser:  depPostgresUser,
			depSecretKeyPass:  pass,
			depSecretKeyDB:    depPostgresDB,
			depSecretKeyDBURL: dbURL,
		},
	}
	return r.Create(ctx, secret)
}

func (r *PreviewEnvironmentReconciler) ensureRedisCredentials(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) error {
	ns := pe.Spec.TargetNamespace
	labels := r.dependencyLabels(pe, depRedisKind, depRedisName)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: depRedisName, Namespace: ns}, existing)
	if err == nil {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
			existing.Labels = labels
			return nil
		})
		return err
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	pass, err := generateDependencyPassword()
	if err != nil {
		return fmt.Errorf("generate redis password: %w", err)
	}
	redisURL := fmt.Sprintf("redis://:%s@%s:%d", pass, depRedisName, depRedisPort)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      depRedisName,
			Namespace: ns,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			depSecretKeyPass:     pass,
			depSecretKeyRedisURL: redisURL,
		},
	}
	return r.Create(ctx, secret)
}

func (r *PreviewEnvironmentReconciler) ensurePostgresDependency(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, spec *miragev1alpha1.PostgresDependencySpec) error {
	if err := r.ensurePostgresCredentials(ctx, pe); err != nil {
		return err
	}
	version := spec.Version
	if version == "" {
		version = defaultPostgresVer
	}
	labels := r.dependencyLabels(pe, depPostgresKind, depPostgresName)
	fsGroup := int64(1001)
	runAsUser := int64(1001)
	container := corev1.Container{
		Name:  depPostgresKind,
		Image: bitnamiPostgresImage + ":" + version,
		Env: []corev1.EnvVar{
			{Name: "POSTGRESQL_USERNAME", Value: depPostgresUser},
			{
				Name: "POSTGRESQL_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: depPostgresName},
						Key:                  depSecretKeyPass,
					},
				},
			},
			{Name: "POSTGRESQL_DATABASE", Value: depPostgresDB},
			{Name: "ALLOW_EMPTY_PASSWORD", Value: "no"},
		},
		Ports: []corev1.ContainerPort{{Name: "postgres", ContainerPort: depPostgresPort}},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/bitnami/postgresql"}},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			RunAsNonRoot:             boolPtr(true),
			RunAsUser:                &runAsUser,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}
	return r.ensureDependencyWorkload(ctx, pe, depPostgresKind, depPostgresName, depPostgresPort, "postgres", labels, container, &corev1.PodSecurityContext{
		RunAsNonRoot:   boolPtr(true),
		RunAsUser:      &runAsUser,
		FSGroup:        &fsGroup,
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}, []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}})
}

func (r *PreviewEnvironmentReconciler) ensureRedisDependency(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, spec *miragev1alpha1.RedisDependencySpec) error {
	if err := r.ensureRedisCredentials(ctx, pe); err != nil {
		return err
	}
	version := spec.Version
	if version == "" {
		version = defaultRedisVer
	}
	labels := r.dependencyLabels(pe, depRedisKind, depRedisName)
	runAsUser := int64(1001)
	fsGroup := int64(1001)
	container := corev1.Container{
		Name:  depRedisKind,
		Image: bitnamiRedisImage + ":" + version,
		Env: []corev1.EnvVar{
			{
				Name: "REDIS_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: depRedisName},
						Key:                  depSecretKeyPass,
					},
				},
			},
			{Name: "ALLOW_EMPTY_PASSWORD", Value: "no"},
			{Name: "REDIS_AOF_ENABLED", Value: "no"},
			{Name: "REDIS_DISABLE_COMMANDS", Value: ""},
		},
		Ports: []corev1.ContainerPort{{Name: "redis", ContainerPort: depRedisPort}},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("25m"), corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/bitnami/redis/data"}},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			RunAsNonRoot:             boolPtr(true),
			RunAsUser:                &runAsUser,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}
	return r.ensureDependencyWorkload(ctx, pe, depRedisKind, depRedisName, depRedisPort, "redis", labels, container, &corev1.PodSecurityContext{
		RunAsNonRoot:   boolPtr(true),
		RunAsUser:      &runAsUser,
		FSGroup:        &fsGroup,
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}, []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}})
}

func (r *PreviewEnvironmentReconciler) ensureKafkaDependency(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment, spec *miragev1alpha1.KafkaDependencySpec) error {
	version := spec.Version
	if version == "" {
		version = defaultKafkaVer
	}
	labels := r.dependencyLabels(pe, depKafkaKind, depKafkaName)
	runAsUser := int64(1001)
	fsGroup := int64(1001)
	container := corev1.Container{
		Name:  depKafkaKind,
		Image: bitnamiKafkaImage + ":" + version,
		Env: []corev1.EnvVar{
			{Name: "KAFKA_CFG_NODE_ID", Value: "0"},
			{Name: "KAFKA_CFG_PROCESS_ROLES", Value: "controller,broker"},
			{Name: "KAFKA_CFG_LISTENERS", Value: "PLAINTEXT://:9092,CONTROLLER://:9093"},
			{Name: "KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP", Value: "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT"},
			{Name: "KAFKA_CFG_CONTROLLER_QUORUM_VOTERS", Value: "0@localhost:9093"},
			{Name: "KAFKA_CFG_CONTROLLER_LISTENER_NAMES", Value: "CONTROLLER"},
			{Name: "KAFKA_CFG_ADVERTISED_LISTENERS", Value: fmt.Sprintf("PLAINTEXT://%s:%d", depKafkaName, depKafkaPort)},
			{Name: "ALLOW_PLAINTEXT_LISTENER", Value: "yes"},
		},
		Ports: []corev1.ContainerPort{{Name: "kafka", ContainerPort: depKafkaPort}},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
		VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/bitnami/kafka"}},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			RunAsNonRoot:             boolPtr(true),
			RunAsUser:                &runAsUser,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}
	return r.ensureDependencyWorkload(ctx, pe, depKafkaKind, depKafkaName, depKafkaPort, "kafka", labels, container, &corev1.PodSecurityContext{
		RunAsNonRoot:   boolPtr(true),
		RunAsUser:      &runAsUser,
		FSGroup:        &fsGroup,
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}, []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}})
}

func (r *PreviewEnvironmentReconciler) ensureDependencyWorkload(
	ctx context.Context,
	pe *miragev1alpha1.PreviewEnvironment,
	kind string,
	name string,
	port int32,
	portName string,
	labels map[string]string,
	container corev1.Container,
	podSC *corev1.PodSecurityContext,
	volumes []corev1.Volume,
) error {
	ns := pe.Spec.TargetNamespace
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Labels = labels
		replicas := int32(1)
		deploy.Spec.Replicas = &replicas
		if deploy.Spec.Selector == nil {
			deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{
				miragev1alpha1.LabelDependency: kind,
				miragev1alpha1.LabelOwnerUID:   string(pe.UID),
			}}
		}
		automount := false
		deploy.Spec.Template.ObjectMeta.Labels = labels
		deploy.Spec.Template.Spec.AutomountServiceAccountToken = &automount
		deploy.Spec.Template.Spec.Containers = []corev1.Container{container}
		deploy.Spec.Template.Spec.SecurityContext = podSC
		deploy.Spec.Template.Spec.Volumes = volumes
		deploy.Spec.RevisionHistoryLimit = int32Ptr(1)
		return nil
	})
	if err != nil {
		return err
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = labels
		svc.Spec.Selector = map[string]string{
			miragev1alpha1.LabelDependency: kind,
			miragev1alpha1.LabelOwnerUID:   string(pe.UID),
		}
		svc.Spec.Ports = []corev1.ServicePort{{
			Name: portName, Port: port, TargetPort: intstr.FromInt32(port), Protocol: corev1.ProtocolTCP,
		}}
		return nil
	})
	return err
}

func (r *PreviewEnvironmentReconciler) dependenciesReady(ctx context.Context, pe *miragev1alpha1.PreviewEnvironment) (bool, string, error) {
	kinds := enabledDependencyKinds(pe.Spec.Dependencies)
	if len(kinds) == 0 {
		return true, "No dependencies configured", nil
	}
	for _, k := range kinds {
		deploy := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: k.Name, Namespace: pe.Spec.TargetNamespace}, deploy)
		if apierrors.IsNotFound(err) {
			return false, fmt.Sprintf("waiting for dependency %s", k.Kind), nil
		}
		if err != nil {
			return false, "", err
		}
		if !deploymentReady(deploy) {
			return false, fmt.Sprintf("waiting for dependency %s", k.Kind), nil
		}
	}
	return true, "All dependencies available", nil
}

func (r *PreviewEnvironmentReconciler) deleteIgnoreNotFound(ctx context.Context, obj client.Object) error {
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
