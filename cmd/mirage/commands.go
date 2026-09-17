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
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	miragev1alpha1 "github.com/sauravrana646/mirage/api/v1alpha1"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "get"},
		Short:   "List PreviewEnvironments",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			var list miragev1alpha1.PreviewEnvironmentList
			opts := []client.ListOption{}
			if namespace != "" {
				opts = append(opts, client.InNamespace(namespace))
			}
			if err := c.List(ctx, &list, opts...); err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tNAMESPACE\tPHASE\tURL\tAGE")
			for _, pe := range list.Items {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					pe.Name, pe.Namespace, pe.Status.Phase, pe.Status.URL, ageString(pe.CreationTimestamp))
			}
			return w.Flush()
		},
	}
}

func newDescribeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "describe NAME",
		Short: "Show PreviewEnvironment details and conditions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			pe, err := getPreview(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Name:           %s\n", pe.Name)
			fmt.Printf("Namespace:      %s\n", pe.Namespace)
			fmt.Printf("Phase:          %s\n", pe.Status.Phase)
			fmt.Printf("URL:            %s\n", pe.Status.URL)
			fmt.Printf("Target NS:      %s\n", pe.Spec.TargetNamespace)
			fmt.Printf("Backend:        %s\n", pe.Spec.Backend)
			fmt.Printf("Template:       %s\n", pe.Status.Template)
			fmt.Printf("Replicas:       %s\n", pe.Status.ReplicaStatus)
			fmt.Printf("Message:        %s\n", pe.Status.Message)
			if pe.Status.ExpiresAt != nil {
				fmt.Printf("Expires:        %s\n", pe.Status.ExpiresAt.Time.Format(timeRFC3339))
			}
			fmt.Println("\nConditions:")
			for _, cond := range pe.Status.Conditions {
				fmt.Printf("  %-16s %s  %s  %s\n", cond.Type, cond.Status, cond.Reason, cond.Message)
			}
			if len(pe.Status.Services) > 0 {
				fmt.Println("\nServices:")
				for _, s := range pe.Status.Services {
					fmt.Printf("  %-12s ready=%v  replicas=%s  url=%s  %s\n", s.Name, s.Ready, s.Replicas, s.URL, s.Message)
				}
			}
			return nil
		},
	}
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func newURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url NAME",
		Short: "Print the preview URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			pe, err := getPreview(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if pe.Status.URL == "" {
				return fmt.Errorf("no URL set (enable ingress or wait for Ready)")
			}
			fmt.Println(pe.Status.URL)
			return nil
		},
	}
}

func newSuspendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "suspend NAME",
		Short: "Suspend reconcile for a preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			if err := setSuspend(cmd.Context(), c, args[0], true); err != nil {
				return err
			}
			fmt.Printf("suspended %s\n", args[0])
			return nil
		},
	}
}

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume NAME",
		Short: "Resume a suspended preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			if err := setSuspend(cmd.Context(), c, args[0], false); err != nil {
				return err
			}
			fmt.Printf("resumed %s\n", args[0])
			return nil
		},
	}
}

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a PreviewEnvironment (triggers cleanup)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			pe, err := getPreview(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if err := c.Delete(cmd.Context(), pe); err != nil {
				return err
			}
			fmt.Printf("deleted %s/%s\n", pe.Namespace, pe.Name)
			return nil
		},
	}
}

func newLogsCmd() *cobra.Command {
	var follow bool
	var service string
	var tail int64
	cmd := &cobra.Command{
		Use:   "logs NAME",
		Short: "Stream logs from preview pods",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, cs, err := kubeClients()
			if err != nil {
				return err
			}
			pe, err := getPreview(ctx, c, args[0])
			if err != nil {
				return err
			}
			selector := "app.kubernetes.io/name=" + pe.Name
			if service != "" {
				selector += "," + miragev1alpha1.LabelService + "=" + service
			}
			pods, err := cs.CoreV1().Pods(pe.Spec.TargetNamespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				return err
			}
			if len(pods.Items) == 0 {
				return fmt.Errorf("no pods in %s matching %s", pe.Spec.TargetNamespace, selector)
			}
			pod := pods.Items[0]
			opts := &corev1.PodLogOptions{Follow: follow}
			if tail > 0 {
				opts.TailLines = &tail
			}
			stream, err := cs.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts).Stream(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = stream.Close() }()
			_, err = io.Copy(os.Stdout, stream)
			return err
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log stream")
	cmd.Flags().StringVar(&service, "service", "", "Service name for multi-service previews")
	cmd.Flags().Int64Var(&tail, "tail", 100, "Lines of recent log to show")
	return cmd
}

func newCreateCmd() *cobra.Command {
	var (
		pr           int
		image        string
		targetNS     string
		templateName string
		ingressHost  string
		ttlSeconds   int64
		createNS     string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a PreviewEnvironment from flags",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if image == "" {
				return fmt.Errorf("--image is required")
			}
			ns := createNS
			if ns == "" {
				ns = namespace
			}
			if ns == "" {
				ns = "default"
			}
			name := fmt.Sprintf("pr-%d", pr)
			if pr <= 0 {
				return fmt.Errorf("--pr is required")
			}
			if targetNS == "" {
				targetNS = "preview-" + name
			}
			pe := &miragev1alpha1.PreviewEnvironment{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec: miragev1alpha1.PreviewEnvironmentSpec{
					Image:           image,
					TargetNamespace: targetNS,
					Source: &miragev1alpha1.SourceSpec{
						Provider:    miragev1alpha1.SCMProviderGitHub,
						PullRequest: pr,
					},
				},
			}
			if ttlSeconds > 0 {
				pe.Spec.TTLSeconds = &ttlSeconds
			}
			if templateName != "" {
				pe.Spec.TemplateRef = &miragev1alpha1.TemplateRef{Name: templateName}
			}
			if ingressHost != "" {
				pe.Spec.Ingress = &miragev1alpha1.IngressSpec{Enabled: true, Host: ingressHost}
			}
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			if err := c.Create(cmd.Context(), pe); err != nil {
				return err
			}
			fmt.Printf("created PreviewEnvironment %s/%s (targetNamespace=%s)\n", ns, name, targetNS)
			return nil
		},
	}
	cmd.Flags().IntVar(&pr, "pr", 0, "Pull request number")
	cmd.Flags().StringVar(&image, "image", "", "Container image")
	cmd.Flags().StringVar(&targetNS, "target-namespace", "", "Preview workload namespace")
	cmd.Flags().StringVar(&templateName, "template", "", "PreviewTemplate name")
	cmd.Flags().StringVar(&ingressHost, "host", "", "Ingress host")
	cmd.Flags().Int64Var(&ttlSeconds, "ttl", 0, "TTL in seconds")
	cmd.Flags().StringVar(&createNS, "cr-namespace", "", "Namespace for the PreviewEnvironment CR")
	return cmd
}

func newDiagnoseCmd() *cobra.Command {
	var aiHint bool
	cmd := &cobra.Command{
		Use:   "diagnose NAME",
		Short: "Explain preview readiness with structured checks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, cs, err := kubeClients()
			if err != nil {
				return err
			}
			pe, err := getPreview(ctx, c, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Preview: %s/%s\n", pe.Namespace, pe.Name)
			fmt.Printf("Status:  %s\n", pe.Status.Phase)
			if pe.Status.Message != "" {
				fmt.Printf("Message: %s\n", pe.Status.Message)
			}
			fmt.Println()

			nsReady := pe.Spec.TargetNamespace != "" &&
				condStatus(pe, miragev1alpha1.ConditionNamespaceReady) != "False"
			printCheck("Namespace created", nsReady, pe.Spec.TargetNamespace)
			netReady := condStatus(pe, miragev1alpha1.ConditionNetworkReady) != "False"
			printCheck("NetworkPolicy applied", netReady, string(pe.Spec.NetworkPolicy))
			wlReady := meta.IsStatusConditionTrue(pe.Status.Conditions, miragev1alpha1.ConditionWorkloadReady) ||
				pe.Status.Phase == miragev1alpha1.PhaseReady
			printCheck("Workload ready", wlReady, pe.Status.ReplicaStatus)
			routeOK := true
			routeCond := meta.FindStatusCondition(pe.Status.Conditions, miragev1alpha1.ConditionRouteReady)
			if routeCond != nil && routeCond.Status == metav1.ConditionFalse {
				routeOK = false
			}
			printCheck("Route ready", routeOK, pe.Status.URL)

			if pe.Status.Phase != miragev1alpha1.PhaseReady {
				fmt.Println("\nDiagnostics:")
				events, err := cs.CoreV1().Events(pe.Spec.TargetNamespace).List(ctx, metav1.ListOptions{Limit: 20})
				if err == nil {
					for _, e := range events.Items {
						if e.Type == corev1.EventTypeWarning {
							fmt.Printf("  Warning %s/%s: %s\n", e.InvolvedObject.Kind, e.InvolvedObject.Name, e.Message)
						}
					}
				}
				pods, err := cs.CoreV1().Pods(pe.Spec.TargetNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: "app.kubernetes.io/name=" + pe.Name,
				})
				if err == nil {
					for _, pod := range pods.Items {
						for _, st := range pod.Status.ContainerStatuses {
							if st.State.Waiting != nil {
								fmt.Printf("  Pod %s waiting: %s — %s\n", pod.Name, st.State.Waiting.Reason, st.State.Waiting.Message)
								suggestForWaiting(st.State.Waiting.Reason, pe)
							}
						}
					}
				}
			}

			if aiHint {
				fmt.Println("\nAI advisory:")
				fmt.Println("  Use the Mirage AI side-path (ai/) with gather → summarize.")
				fmt.Println("  LLM calls stay outside the reconcile loop; pass status/events/logs as context.")
				if pe.Annotations != nil {
					if s := pe.Annotations[miragev1alpha1.AnnotationAISummary]; s != "" {
						fmt.Printf("  Existing summary: %s\n", s)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&aiHint, "ai", false, "Show how to use the AI advisory path")
	return cmd
}

func printCheck(label string, ok bool, detail string) {
	mark := "✗"
	if ok {
		mark = "✓"
	}
	if detail != "" {
		fmt.Printf("%s %s (%s)\n", mark, label, detail)
		return
	}
	fmt.Printf("%s %s\n", mark, label)
}

func suggestForWaiting(reason string, pe *miragev1alpha1.PreviewEnvironment) {
	ns := pe.Spec.TargetNamespace
	switch reason {
	case "ImagePullBackOff", "ErrImagePull":
		fmt.Println("  Possible cause: registry credentials unavailable in preview namespace.")
		fmt.Println("  Suggested checks:")
		fmt.Printf("    kubectl get events -n %s\n", ns)
		fmt.Printf("    kubectl get secret -n %s\n", ns)
	case "CrashLoopBackOff":
		fmt.Println("  Possible cause: application crash on startup.")
		fmt.Printf("    mirage logs %s\n", pe.Name)
	default:
		fmt.Printf("    kubectl describe pod -n %s -l app.kubernetes.io/name=%s\n", ns, pe.Name)
	}
}
