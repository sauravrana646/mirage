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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func restConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	)
	cfg, err := loader.ClientConfig()
	if err == nil {
		return cfg, nil
	}
	home, _ := os.UserHomeDir()
	return clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
}

func kubeClients() (client.Client, kubernetes.Interface, error) {
	cfg, err := restConfig()
	if err != nil {
		return nil, nil, err
	}
	scheme := runtime.NewScheme()
	if err := miragev1alpha1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return c, cs, nil
}

func getPreview(ctx context.Context, c client.Client, name string) (*miragev1alpha1.PreviewEnvironment, error) {
	if namespace != "" {
		pe := &miragev1alpha1.PreviewEnvironment{}
		if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, pe); err != nil {
			return nil, err
		}
		return pe, nil
	}
	var list miragev1alpha1.PreviewEnvironmentList
	if err := c.List(ctx, &list); err != nil {
		return nil, err
	}
	var matches []miragev1alpha1.PreviewEnvironment
	for i := range list.Items {
		if list.Items[i].Name == name {
			matches = append(matches, list.Items[i])
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("PreviewEnvironment %q not found", name)
	}
	if len(matches) > 1 {
		var nss []string
		for _, m := range matches {
			nss = append(nss, m.Namespace)
		}
		return nil, fmt.Errorf(
			"multiple PreviewEnvironments named %q in namespaces %s; pass -n",
			name, strings.Join(nss, ", "),
		)
	}
	return &matches[0], nil
}

// getTemplateForPreview loads the PreviewTemplate when pe.Spec.TemplateRef is set.
// Returns (nil, nil) when no template is referenced.
func getTemplateForPreview(
	ctx context.Context, c client.Client, pe *miragev1alpha1.PreviewEnvironment,
) (*miragev1alpha1.PreviewTemplate, error) {
	if pe == nil || pe.Spec.TemplateRef == nil || pe.Spec.TemplateRef.Name == "" {
		return nil, nil
	}
	tplNS := pe.Spec.TemplateRef.Namespace
	if tplNS == "" {
		tplNS = pe.Namespace
	}
	tpl := &miragev1alpha1.PreviewTemplate{}
	if err := c.Get(ctx, types.NamespacedName{Name: pe.Spec.TemplateRef.Name, Namespace: tplNS}, tpl); err != nil {
		return nil, fmt.Errorf("load PreviewTemplate %s/%s: %w", tplNS, pe.Spec.TemplateRef.Name, err)
	}
	return tpl, nil
}

func condStatus(pe *miragev1alpha1.PreviewEnvironment, typ string) string {
	c := meta.FindStatusCondition(pe.Status.Conditions, typ)
	if c == nil {
		return "-"
	}
	return string(c.Status)
}

func ageString(t metav1.Time) string {
	d := time.Since(t.Time).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func setSuspend(ctx context.Context, c client.Client, name string, suspend bool) error {
	pe, err := getPreview(ctx, c, name)
	if err != nil {
		return err
	}
	pe.Spec.Suspend = suspend
	return c.Update(ctx, pe)
}
