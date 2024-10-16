/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2021 Red Hat, Inc.
 *
 */
package tests

import (
	"context"
	"fmt"

	"kubevirt.io/kubevirt/tests/framework/kubevirt"
	"kubevirt.io/kubevirt/tests/libnode"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/tests/exec"
	"kubevirt.io/kubevirt/tests/flags"
)

func NodeNameWithHandler() string {
	listOptions := metav1.ListOptions{LabelSelector: v1.AppLabel + "=virt-handler"}
	virtClient := kubevirt.Client()
	virtHandlerPods, err := virtClient.CoreV1().Pods(flags.KubeVirtInstallNamespace).List(context.Background(), listOptions)
	Expect(err).ToNot(HaveOccurred())
	node, err := virtClient.CoreV1().Nodes().Get(context.Background(), virtHandlerPods.Items[0].Spec.NodeName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	return node.ObjectMeta.Name
}

func ExecuteCommandInVirtHandlerPod(nodeName string, args []string) (stdout string, err error) {
	virtClient := kubevirt.Client()

	pod, err := libnode.GetVirtHandlerPod(virtClient, nodeName)
	if err != nil {
		return stdout, err
	}

	stdout, stderr, err := exec.ExecuteCommandOnPodWithResults(pod, "virt-handler", args)
	if err != nil {
		return stdout, fmt.Errorf("Failed excuting command=%v, error=%v, stdout=%s, stderr=%s", args, err, stdout, stderr)
	}
	return stdout, nil
}
