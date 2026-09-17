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
	"testing"

	corev1 "k8s.io/api/core/v1"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func TestDependencyEnvVars(t *testing.T) {
	if got := dependencyEnvVars(nil); len(got) != 0 {
		t.Fatalf("expected nil/empty for nil deps, got %+v", got)
	}
	deps := &miragev1alpha1.PreviewDependenciesSpec{
		Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: true},
		Redis:    &miragev1alpha1.RedisDependencySpec{Enabled: true},
		Kafka:    &miragev1alpha1.KafkaDependencySpec{Enabled: true},
	}
	got := dependencyEnvVars(deps)
	if len(got) != 3 {
		t.Fatalf("expected 3 env vars, got %+v", got)
	}
	byName := map[string]corev1.EnvVar{}
	for _, e := range got {
		byName[e.Name] = e
	}
	db := byName["DATABASE_URL"]
	if db.Value != "" || db.ValueFrom == nil || db.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("DATABASE_URL should use secretKeyRef, got %+v", db)
	}
	if db.ValueFrom.SecretKeyRef.Name != depPostgresName || db.ValueFrom.SecretKeyRef.Key != depSecretKeyDBURL {
		t.Fatalf("DATABASE_URL secret ref: got %s/%s", db.ValueFrom.SecretKeyRef.Name, db.ValueFrom.SecretKeyRef.Key)
	}
	redis := byName["REDIS_URL"]
	if redis.Value != "" || redis.ValueFrom == nil || redis.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("REDIS_URL should use secretKeyRef, got %+v", redis)
	}
	if redis.ValueFrom.SecretKeyRef.Name != depRedisName || redis.ValueFrom.SecretKeyRef.Key != depSecretKeyRedisURL {
		t.Fatalf("REDIS_URL secret ref: got %s/%s", redis.ValueFrom.SecretKeyRef.Name, redis.ValueFrom.SecretKeyRef.Key)
	}
	if byName["KAFKA_BROKERS"].Value != "mirage-dep-kafka:9092" {
		t.Errorf("KAFKA_BROKERS: got %q", byName["KAFKA_BROKERS"].Value)
	}
}

func TestDependencyEnvVarsDisabledSkipped(t *testing.T) {
	deps := &miragev1alpha1.PreviewDependenciesSpec{
		Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: false},
		Redis:    &miragev1alpha1.RedisDependencySpec{Enabled: true},
	}
	got := dependencyEnvVars(deps)
	if len(got) != 1 || got[0].Name != "REDIS_URL" {
		t.Fatalf("expected only REDIS_URL, got %+v", got)
	}
	if got[0].ValueFrom == nil || got[0].ValueFrom.SecretKeyRef == nil {
		t.Fatalf("REDIS_URL should use secretKeyRef")
	}
}

func TestMergeEnvPreferUser(t *testing.T) {
	user := []corev1.EnvVar{{Name: "DATABASE_URL", Value: "user-override"}, {Name: "APP", Value: "1"}}
	injected := dependencyEnvVars(&miragev1alpha1.PreviewDependenciesSpec{
		Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: true},
		Redis:    &miragev1alpha1.RedisDependencySpec{Enabled: true},
	})
	merged := mergeEnvPreferUser(user, injected)
	byName := map[string]corev1.EnvVar{}
	for _, e := range merged {
		byName[e.Name] = e
	}
	if byName["DATABASE_URL"].Value != "user-override" {
		t.Fatalf("user DATABASE_URL should win, got %+v", byName["DATABASE_URL"])
	}
	redis := byName["REDIS_URL"]
	if redis.ValueFrom == nil || redis.ValueFrom.SecretKeyRef == nil || redis.ValueFrom.SecretKeyRef.Name != depRedisName {
		t.Fatalf("expected injected REDIS_URL secret ref, got %+v", redis)
	}
	if byName["APP"].Value != "1" {
		t.Fatalf("expected APP preserved")
	}
}

func TestDependenciesEnabled(t *testing.T) {
	if dependenciesEnabled(nil) {
		t.Fatal("nil should be disabled")
	}
	if dependenciesEnabled(&miragev1alpha1.PreviewDependenciesSpec{}) {
		t.Fatal("empty should be disabled")
	}
	if !dependenciesEnabled(&miragev1alpha1.PreviewDependenciesSpec{
		Redis: &miragev1alpha1.RedisDependencySpec{Enabled: true},
	}) {
		t.Fatal("redis enabled should be true")
	}
}

func TestDependencyResourceNames(t *testing.T) {
	if depPostgresName != "mirage-dep-postgres" || depRedisName != "mirage-dep-redis" || depKafkaName != "mirage-dep-kafka" {
		t.Fatalf("unexpected dep resource names: %s %s %s", depPostgresName, depRedisName, depKafkaName)
	}
	kinds := enabledDependencyKinds(&miragev1alpha1.PreviewDependenciesSpec{
		Postgres: &miragev1alpha1.PostgresDependencySpec{Enabled: true},
		Redis:    &miragev1alpha1.RedisDependencySpec{Enabled: true},
		Kafka:    &miragev1alpha1.KafkaDependencySpec{Enabled: true},
	})
	if len(kinds) != 3 {
		t.Fatalf("expected 3 kinds, got %d", len(kinds))
	}
	if kinds[0].Kind != depPostgresKind || kinds[0].Name != depPostgresName {
		t.Fatalf("postgres kind/name: %+v", kinds[0])
	}
}

func TestGenerateDependencyPassword(t *testing.T) {
	a, err := generateDependencyPassword()
	if err != nil {
		t.Fatal(err)
	}
	b, err := generateDependencyPassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != depPasswordBytes || len(b) != depPasswordBytes {
		t.Fatalf("expected length %d", depPasswordBytes)
	}
	if a == b {
		t.Fatal("expected distinct passwords")
	}
	for _, c := range a {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Fatalf("non-alphanumeric char in password: %q", c)
		}
	}
}
