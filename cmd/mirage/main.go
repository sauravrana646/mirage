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
	"os"

	"github.com/spf13/cobra"
)

var (
	kubeconfig string
	namespace  string
)

func main() {
	root := &cobra.Command{
		Use:   "mirage",
		Short: "CLI for Mirage preview environments",
		Long:  "Inspect, manage, and diagnose PreviewEnvironment resources without hand-writing kubectl.",
	}
	root.PersistentFlags().StringVar(
		&kubeconfig, "kubeconfig", "",
		"Path to kubeconfig (defaults to KUBECONFIG or ~/.kube/config)",
	)
	root.PersistentFlags().StringVarP(
		&namespace, "namespace", "n", "",
		"Namespace of the PreviewEnvironment CR (default: all for list)",
	)

	root.AddCommand(
		newListCmd(),
		newDescribeCmd(),
		newLogsCmd(),
		newURLCmd(),
		newSuspendCmd(),
		newResumeCmd(),
		newDeleteCmd(),
		newDiagnoseCmd(),
		newCreateCmd(),
		newCostCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
