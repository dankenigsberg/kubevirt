package components

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pluginv1alpha1 "kubevirt.io/api/plugin/v1alpha1"

	"kubevirt.io/kubevirt/pkg/pointer"
	"kubevirt.io/kubevirt/pkg/util"
)

func NewRootLauncherPlugin() *pluginv1alpha1.Plugin {
	rootUser := int64(util.RootUser)
	return &pluginv1alpha1.Plugin{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "plugin.kubevirt.io/v1alpha1",
			Kind:       "Plugin",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "root-launcher",
		},
		Spec: pluginv1alpha1.PluginSpec{
			Condition:       `has(vmi.metadata.annotations) && "kubevirt.io/nonroot" in vmi.metadata.annotations && vmi.metadata.annotations["kubevirt.io/nonroot"] == "false"`,
			FailureStrategy: pluginv1alpha1.FailureStrategyFail,
			LauncherPodHooks: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    &rootUser,
						RunAsGroup:   &rootUser,
						RunAsNonRoot: pointer.P(false),
					},
					Containers: []corev1.Container{
						{
							Name: "compute",
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"SYS_NICE"},
								},
								AllowPrivilegeEscalation: pointer.P(true),
							},
							Env: []corev1.EnvVar{
								{Name: "VIRT_LAUNCHER_LIBVIRT_URI", Value: "qemu+unix:///system"},
								{Name: "VIRT_LAUNCHER_LOG_DIR", Value: "/var/log/libvirt/qemu/"},
								{Name: "VIRT_LAUNCHER_PID_DIR", Value: "/run/libvirt/qemu/run"},
								{Name: "VIRT_LAUNCHER_SWTPM_DIR", Value: "/var/lib/swtpm-localca"},
								{Name: "VIRT_LAUNCHER_CBT_DIR", Value: "/var/run/kubevirt-private/libvirt/qemu/cbt"},
								{Name: "VIRT_LAUNCHER_UID", Value: "0"},
							},
						},
					},
				},
			},
		},
	}
}
