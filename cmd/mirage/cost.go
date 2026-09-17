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
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/sauravrana646/mirage/internal/cost"
)

func newCostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cost NAME",
		Short: "Estimate preview cost from requested resources and age",
		Long: `Estimate spend for a PreviewEnvironment using hardcoded illustrative rates
(CPU/memory/ingress $/hour). When templateRef is set, loads the PreviewTemplate
and applies the same defaults as the controller (presets, replicas, ingress).
Enabled dependencies (postgres/redis/kafka) add fixed request footprints.
This is not cloud billing — OpenCost or similar can replace it later.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := kubeClients()
			if err != nil {
				return err
			}
			pe, err := getPreview(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			tpl, err := getTemplateForPreview(cmd.Context(), c, pe)
			if err != nil {
				return err
			}
			est := cost.EstimateCostFrom(pe, tpl, time.Now())
			printCostEstimate(est)
			return nil
		},
	}
}

func printCostEstimate(est cost.Estimate) {
	cpuQ := *resource.NewMilliQuantity(est.Resources.CPUMillis, resource.DecimalSI)
	memQ := *resource.NewQuantity(est.Resources.MemoryBytes, resource.BinarySI)
	storage := "0 (not tracked)"
	if est.Resources.StorageBytes > 0 {
		sq := *resource.NewQuantity(est.Resources.StorageBytes, resource.BinarySI)
		storage = sq.String()
	}
	ingress := "disabled"
	if est.Resources.IngressCount > 0 {
		ingress = fmt.Sprintf("%d enabled", est.Resources.IngressCount)
	}

	fmt.Println("Estimated preview cost")
	fmt.Printf("CPU       %s (%.3f cores @ $%.4f/core-hour)\n",
		cpuQ.String(), est.Resources.CPUCores(), cost.CPUHourlyUSDPerCore)
	fmt.Printf("Memory    %s (%.3f GiB @ $%.4f/GiB-hour)\n",
		memQ.String(), est.Resources.MemoryGiB(), cost.MemoryHourlyUSDPerGiB)
	fmt.Printf("Storage   %s\n", storage)
	fmt.Printf("Ingress   %s (@ $%.4f/hour each)\n", ingress, cost.IngressHourlyUSD)
	fmt.Printf("Running:  %s (%.2f hours)\n", formatDuration(est.Age), est.Hours)
	fmt.Printf("Estimated: $%.2f\n", est.TotalUSD)
	if est.ExpiresAt != nil {
		fmt.Printf("Expires:  %s\n", est.ExpiresAt.Format(timeRFC3339))
	} else {
		fmt.Println("Expires:  (none)")
	}
	fmt.Println()
	fmt.Println("Note: rates are illustrative estimates, not invoices.")
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
