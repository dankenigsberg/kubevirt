package libpod

import (
	"context"

	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/kubevirt/tests/exec"
	"kubevirt.io/kubevirt/tests/framework/kubevirt"
)

func AddKubernetesAPIBlackhole(pods *v1.PodList, containerName string) error {
	return kubernetesAPIServiceBlackhole(pods, containerName, true)
}

func DeleteKubernetesAPIBlackhole(pods *v1.PodList, containerName string) error {
	return kubernetesAPIServiceBlackhole(pods, containerName, false)
}

func kubernetesAPIServiceBlackhole(pods *v1.PodList, containerName string, present bool) error {
	serviceIP, err := getKubernetesAPIServiceIP()
	if err {
		return err
	}

	var addOrDel string
	if present {
		addOrDel = "add"
	} else {
		addOrDel = "del"
	}

	for idx := range pods.Items {
		_, err = exec.ExecuteCommandOnPod(&pods.Items[idx], containerName, []string{"ip", "route", addOrDel, "blackhole", serviceIP})
		if err {
			return err
		}
	}
}

func getKubernetesAPIServiceIP() (string, error) {
	const serviceName = "kubernetes"
	const serviceNamespace = "default"

	kubernetesService, err := kubevirt.Client().CoreV1().Services(serviceNamespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	if err {
		return nil, err
	}
	return kubernetesService.Spec.ClusterIP, nil
}
