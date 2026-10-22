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
 * Copyright The KubeVirt Authors.
 *
 */

package cloudinit_test

import (
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "kubevirt.io/api/core/v1"

	cloudinit "kubevirt.io/kubevirt/pkg/cloud-init"
	"kubevirt.io/kubevirt/pkg/libvmi"
	cloudinitdata "kubevirt.io/kubevirt/pkg/libvmi/cloudinit"
)

var _ = Describe("ResolveVolumeData", func() {
	It("returns nil when the VMI has no cloud-init volume", func() {
		vmi := libvmi.New()
		data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(BeNil())
	})
	Context("NoCloud", func() {
		It("reads raw user and network payloads", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
					cloudinitdata.WithNoCloudNetworkData("network"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.DataSource).To(Equal(cloudinit.DataSourceNoCloud))
			Expect(data.UserData).To(Equal("user"))
			Expect(data.NetworkData).To(Equal("network"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
			Expect(cloudinit.IsValidCloudInitData(data)).To(BeTrue())
		})

		It("decodes base64 user and network payloads", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudEncodedUserData("user"),
					cloudinitdata.WithNoCloudEncodedNetworkData("network"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("user"))
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("prefers raw payloads to malformed base64", func() {
			// Preserve existing raw-data precedence during this refactor.
			// Returning an error for malformed base64 would be more prudent
			// than silently ignoring it when raw data is also present.
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
					cloudinitdata.WithNoCloudNetworkData("network"),
				),
			)
			volume := libvmi.GetCloudInitVolume(vmi).CloudInitNoCloud
			volume.UserDataBase64 = "!"
			volume.NetworkDataBase64 = "!"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("user"))
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("accepts network-only source data", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudNetworkData("network"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(BeEmpty())
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("rejects empty source data", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("userDataBase64, userData, networkDataBase64 or networkData is required for a cloud-init data source"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
		})

		It("rejects malformed user base64", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudNetworkData("network"),
				),
			)
			libvmi.GetCloudInitVolume(vmi).CloudInitNoCloud.UserDataBase64 = "!"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("illegal base64 data at input byte 0"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
		})

		It("rejects malformed network base64", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)
			libvmi.GetCloudInitVolume(vmi).CloudInitNoCloud.NetworkDataBase64 = "!"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("illegal base64 data at input byte 0"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
		})

		It("uses the firmware UUID in metadata", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.InstanceID).To(Equal("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"))
		})

		It("prefers the cluster instancetype annotation to the namespaced annotation", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithAnnotation(v1.InstancetypeAnnotation, "namespaced"),
				libvmi.WithAnnotation(v1.ClusterInstancetypeAnnotation, "cluster"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.InstanceType).To(Equal("cluster"))
		})

		It("uses the namespaced instancetype when no cluster annotation exists", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithAnnotation(v1.InstancetypeAnnotation, "namespaced"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.InstanceType).To(Equal("namespaced"))
		})

		It("preserves an explicitly empty firmware UUID when reading", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID(""),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.InstanceID).To(BeEmpty())
		})

		It("uses userData when userdata cannot be read", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserDataSecretName("secret"),
				),
			)
			secretData := cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/userdata": {Err: errors.New("unreadable")},
				"cloudinitdisk/userData": {Data: "secret-user"},
			}}

			data, err := cloudinit.ResolveVolumeData(vmi, secretData)

			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("secret-user"))
		})

		It("prefers lowercase networkdata when both network filenames contain data", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudNetworkDataSecretName("secret"),
				),
			)
			secretData := cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/networkdata": {Data: "primary-network"},
				"cloudinitdisk/networkData": {Data: "secondary-network"},
			}}

			data, err := cloudinit.ResolveVolumeData(vmi, secretData)

			Expect(err).NotTo(HaveOccurred())
			Expect(data.NetworkData).To(Equal("primary-network"))
		})

		It("keeps inline data when the only requested secret is missing", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("inline"),
					cloudinitdata.WithNoCloudUserDataSecretName("missing-user-secret"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("inline"))
		})

		It("rejects two missing secret payloads", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("inline"),
					cloudinitdata.WithNoCloudUserDataSecretName("missing-user-secret"),
					cloudinitdata.WithNoCloudNetworkDataSecretName("missing-network-secret"),
				),
			)
			_, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("no cloud-init data-source found at volume: cloudinitdisk"))
		})

		It("accepts a network secret when the user secret is missing", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("inline"),
					cloudinitdata.WithNoCloudUserDataSecretName("missing-user-secret"),
					cloudinitdata.WithNoCloudNetworkDataSecretName("network-secret"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/networkdata": {Data: "network"},
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("inline"))
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("prefers lowercase cloudinitdisk/userdata even if empty", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("inline"),
					cloudinitdata.WithNoCloudUserDataSecretName("secret"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/userdata": {},
				"cloudinitdisk/userData": {Data: "ignored"},
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("inline"))
		})

		It("orders selected public keys and ignores projected bookkeeping", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
				withNoCloudSSHPublicKey("first"),
				withConfigDriveSSHPublicKey("ignored"),
				withAccessCredential(v1.AccessCredential{}),
				withNoCloudSSHPublicKey(""),
				withNoCloudSSHPublicKey("second"),
			)

			secretData := cloudinit.SecretData{Directories: map[string]cloudinit.SecretDirectory{
				"first-access-cred": {Entries: []cloudinit.SecretEntry{
					{Name: "..data", Err: errors.New("ignored bookkeeping")},
					{Name: "dir", IsDir: true, Err: errors.New("ignored directory")},
					{Name: "a", Data: "first"}, {Name: "empty"}, {Name: "z", Data: "last"},
				}},
				"second-access-cred": {Entries: []cloudinit.SecretEntry{{Name: "link", Data: "second"}}},
			}}
			data, err := cloudinit.ResolveVolumeData(vmi, secretData)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.PublicSSHKeys).To(Equal(map[string]string{"0": "first", "1": "last", "2": "second"}))
		})

		It("returns a credential directory read error", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
				withNoCloudSSHPublicKey("keys"),
			)

			readErr := errors.New("permission denied")
			_, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{Directories: map[string]cloudinit.SecretDirectory{
				"keys-access-cred": {Err: readErr},
			}})
			Expect(err).To(MatchError(readErr))
		})

		It("reports the path of a credential file read error", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
				withNoCloudSSHPublicKey("keys"),
			)

			_, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{
				SourceDir: "/mounted",
				Directories: map[string]cloudinit.SecretDirectory{
					"keys-access-cred": {Entries: []cloudinit.SecretEntry{{Name: "public", Err: errors.New("permission denied")}}},
				},
			})
			Expect(err).To(MatchError("Unable to read public keys found at volume: /mounted/keys-access-cred/public error: permission denied"))
		})

		It("uses the VMI name without its domain suffix as the metadata hostname", func() {
			vmi := libvmi.New(
				libvmi.WithName("guest.example.com"),
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.LocalHostname).To(Equal("guest"))
		})

		It("uses the VMI name truncated to 63 characters as the metadata hostname", func() {
			// Preserve existing silent truncation during this refactor. Returning
			// an error would be more prudent than discarding the hostname's tail.
			vmi := libvmi.New(
				libvmi.WithName(strings.Repeat("a", 70)),
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.LocalHostname).To(Equal(strings.Repeat("a", 63)))
		})

		It("honors an explicitly configured hostname", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserData("user"),
				),
			)
			vmi.Spec.Hostname = "guest-host"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.NoCloudMetaData.LocalHostname).To(Equal("guest-host"))
		})
	})
	Context("ConfigDrive", func() {
		It("reads raw user and network payloads", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
					cloudinitdata.WithConfigDriveNetworkData("network"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.DataSource).To(Equal(cloudinit.DataSourceConfigDrive))
			Expect(data.UserData).To(Equal("user"))
			Expect(data.NetworkData).To(Equal("network"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
		})

		It("decodes base64 user and network payloads", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveEncodedUserData("user"),
					cloudinitdata.WithConfigDriveEncodedNetworkData("network"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("user"))
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("prefers raw payloads to malformed base64", func() {
			// Preserve existing raw-data precedence during this refactor.
			// Returning an error for malformed base64 would be more prudent
			// than silently ignoring it when raw data is also present.
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
					cloudinitdata.WithConfigDriveNetworkData("network"),
				),
			)
			volume := libvmi.GetCloudInitVolume(vmi).CloudInitConfigDrive
			volume.UserDataBase64 = "!"
			volume.NetworkDataBase64 = "!"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("user"))
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("accepts network-only source data", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveNetworkData("network"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(BeEmpty())
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("rejects empty source data", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("userDataBase64, userData, networkDataBase64 or networkData is required for a cloud-init data source"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
		})

		It("rejects malformed user base64", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveNetworkData("network"),
				),
			)
			libvmi.GetCloudInitVolume(vmi).CloudInitConfigDrive.UserDataBase64 = "!"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("illegal base64 data at input byte 0"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
		})

		It("rejects malformed network base64", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)
			libvmi.GetCloudInitVolume(vmi).CloudInitConfigDrive.NetworkDataBase64 = "!"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("illegal base64 data at input byte 0"))
			Expect(data.VolumeName).To(Equal(libvmi.CloudInitDiskName))
		})

		It("uses the firmware UUID in metadata", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.UUID).To(Equal("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"))
		})

		It("prefers the cluster instancetype annotation to the namespaced annotation", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithAnnotation(v1.InstancetypeAnnotation, "namespaced"),
				libvmi.WithAnnotation(v1.ClusterInstancetypeAnnotation, "cluster"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.InstanceType).To(Equal("cluster"))
		})

		It("uses the namespaced instancetype when no cluster annotation exists", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithAnnotation(v1.InstancetypeAnnotation, "namespaced"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.InstanceType).To(Equal("namespaced"))
		})

		It("preserves an explicitly empty firmware UUID when reading", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID(""),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.UUID).To(BeEmpty())
		})

		It("uses userData when userdata cannot be read", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserDataSecretName("secret"),
				),
			)
			secretData := cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/userdata": {Err: errors.New("unreadable")},
				"cloudinitdisk/userData": {Data: "secret-user"},
			}}

			data, err := cloudinit.ResolveVolumeData(vmi, secretData)

			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("secret-user"))
		})

		It("prefers networkdata when both network filenames contain data", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveNetworkDataSecretName("secret"),
				),
			)
			secretData := cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/networkdata": {Data: "primary-network"},
				"cloudinitdisk/networkData": {Data: "secondary-network"},
			}}

			data, err := cloudinit.ResolveVolumeData(vmi, secretData)

			Expect(err).NotTo(HaveOccurred())
			Expect(data.NetworkData).To(Equal("primary-network"))
		})

		It("keeps inline data when the only requested secret is missing", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("inline"),
					cloudinitdata.WithConfigDriveUserDataSecretName("missing-user-secret"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("inline"))
		})

		It("rejects two missing secret payloads", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("inline"),
					cloudinitdata.WithConfigDriveUserDataSecretName("missing-user-secret"),
					cloudinitdata.WithConfigDriveNetworkDataSecretName("missing-network-secret"),
				),
			)
			_, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).To(MatchError("no cloud-init data-source found at volume: cloudinitdisk"))
		})

		It("accepts a network secret when the user secret is missing", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("inline"),
					cloudinitdata.WithConfigDriveUserDataSecretName("missing-user-secret"),
					cloudinitdata.WithConfigDriveNetworkDataSecretName("network-secret"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/networkdata": {Data: "network"},
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("inline"))
			Expect(data.NetworkData).To(Equal("network"))
		})

		It("prefers lowercase cloudinitdisk/userdata even if empty", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("inline"),
					cloudinitdata.WithConfigDriveUserDataSecretName("secret"),
				),
			)
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{
				"cloudinitdisk/userdata": {},
				"cloudinitdisk/userData": {Data: "ignored"},
			}})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.UserData).To(Equal("inline"))
		})

		It("orders selected public keys and ignores projected bookkeeping", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
				withConfigDriveSSHPublicKey("first"),
				withNoCloudSSHPublicKey("ignored"),
				withAccessCredential(v1.AccessCredential{}),
				withConfigDriveSSHPublicKey(""),
				withConfigDriveSSHPublicKey("second"),
			)

			secretData := cloudinit.SecretData{Directories: map[string]cloudinit.SecretDirectory{
				"first-access-cred": {Entries: []cloudinit.SecretEntry{
					{Name: "..data", Err: errors.New("ignored bookkeeping")},
					{Name: "dir", IsDir: true, Err: errors.New("ignored directory")},
					{Name: "a", Data: "first"}, {Name: "empty"}, {Name: "z", Data: "last"},
				}},
				"second-access-cred": {Entries: []cloudinit.SecretEntry{{Name: "link", Data: "second"}}},
			}}
			data, err := cloudinit.ResolveVolumeData(vmi, secretData)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.PublicSSHKeys).To(Equal(map[string]string{"0": "first", "1": "last", "2": "second"}))
		})

		It("returns a credential directory read error", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
				withConfigDriveSSHPublicKey("keys"),
			)

			readErr := errors.New("permission denied")
			_, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{Directories: map[string]cloudinit.SecretDirectory{
				"keys-access-cred": {Err: readErr},
			}})
			Expect(err).To(MatchError(readErr))
		})

		It("reports the path of a credential file read error", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
				withConfigDriveSSHPublicKey("keys"),
			)

			_, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{
				SourceDir: "/mounted",
				Directories: map[string]cloudinit.SecretDirectory{
					"keys-access-cred": {Entries: []cloudinit.SecretEntry{{Name: "public", Err: errors.New("permission denied")}}},
				},
			})
			Expect(err).To(MatchError("Unable to read public keys found at volume: /mounted/keys-access-cred/public error: permission denied"))
		})

		It("uses the VMI name without its domain suffix as the metadata hostname", func() {
			vmi := libvmi.New(
				libvmi.WithName("guest.example.com"),
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.Hostname).To(Equal("guest"))
		})

		It("uses the VMI name truncated to 63 characters as the metadata hostname", func() {
			// Preserve existing silent truncation during this refactor. Returning
			// an error would be more prudent than discarding the hostname's tail.
			vmi := libvmi.New(
				libvmi.WithName(strings.Repeat("a", 70)),
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)

			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})

			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.Hostname).To(Equal(strings.Repeat("a", 63)))
		})

		It("honors an explicitly configured hostname", func() {
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserData("user"),
				),
			)
			vmi.Spec.Hostname = "guest-host"
			data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{})
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ConfigDriveMetaData.Hostname).To(Equal("guest-host"))
		})
	})
	It("returns data from the first cloud-init volume without changing later volumes", func() {
		vmi := libvmi.New(
			libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
			libvmi.WithCloudInitNoCloud(cloudinitdata.WithNoCloudUserData("first")),
		)
		second := libvmi.New(libvmi.WithCloudInitNoCloud(
			cloudinitdata.WithNoCloudUserData("second"),
			cloudinitdata.WithNoCloudUserDataSecretName("secret"),
		))
		secondVolume := *libvmi.GetCloudInitVolume(second)
		secondVolume.Name = "second"
		third := libvmi.New(libvmi.WithCloudInitConfigDrive(
			cloudinitdata.WithConfigDriveUserData("third"),
		))
		thirdVolume := *libvmi.GetCloudInitVolume(third)
		thirdVolume.Name = "third"
		vmi.Spec.Volumes = append(vmi.Spec.Volumes, secondVolume, thirdVolume)
		original := vmi.DeepCopy()
		data, err := cloudinit.ResolveVolumeData(vmi, cloudinit.SecretData{Files: map[string]cloudinit.SecretFile{"second/userdata": {Data: "resolved"}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(data.UserData).To(Equal("first"))
		Expect(data.DataSource).To(Equal(cloudinit.DataSourceNoCloud))
		Expect(vmi).To(Equal(original))
	})

})

var _ = Describe("BuildIsoPlan", func() {
	Context("NoCloud", func() {
		It("builds the exact ISO layout and serialized metadata", func() {
			data := &cloudinit.CloudInitData{
				DataSource:  cloudinit.DataSourceNoCloud,
				UserData:    "user",
				NetworkData: "network",
				NoCloudMetaData: &cloudinit.NoCloudMetadata{
					InstanceID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5", LocalHostname: "vm",
				},
			}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan).To(Equal(&cloudinit.IsoPlan{
				VolumeID: "cidata", FileName: "noCloud.iso", DataDir: "",
				Files: []cloudinit.IsoFile{
					{Path: "user-data", Data: []byte("user")},
					{Path: "meta-data", Data: []byte(`{"instance-id":"cf73b4f4-6975-4d93-b51c-b7ae392bdea5","local-hostname":"vm"}`)},
					{Path: "network-config", Data: []byte("network")},
				},
			}))
		})

		It("omits the network file when no network data is supplied", func() {
			data := &cloudinit.CloudInitData{
				DataSource: cloudinit.DataSourceNoCloud,
				UserData:   "user",
				NoCloudMetaData: &cloudinit.NoCloudMetadata{
					InstanceID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5", LocalHostname: "vm",
				},
			}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Files).To(Equal([]cloudinit.IsoFile{
				{Path: "user-data", Data: []byte("user")},
				{Path: "meta-data", Data: []byte(`{"instance-id":"cf73b4f4-6975-4d93-b51c-b7ae392bdea5","local-hostname":"vm"}`)},
			}))
		})

		It("includes an empty user-data file for network-only input", func() {
			data := &cloudinit.CloudInitData{DataSource: cloudinit.DataSourceNoCloud, NetworkData: "network", NoCloudMetaData: &cloudinit.NoCloudMetadata{InstanceID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5"}}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Files[0]).To(Equal(cloudinit.IsoFile{Path: "user-data", Data: []byte{}}))
			Expect(plan.Files[2]).To(Equal(cloudinit.IsoFile{Path: "network-config", Data: []byte("network")}))
		})

		It("rejects generation without user or network data", func() {
			data := &cloudinit.CloudInitData{DataSource: cloudinit.DataSourceNoCloud}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).To(MatchError("UserData or NetworkData is required for cloud-init data source"))
			Expect(plan).NotTo(BeNil())
		})

	})
	Context("ConfigDrive", func() {
		It("builds the exact ISO layout and serialized metadata", func() {
			data := &cloudinit.CloudInitData{
				DataSource:  cloudinit.DataSourceConfigDrive,
				UserData:    "user",
				NetworkData: "network",
				ConfigDriveMetaData: &cloudinit.ConfigDriveMetadata{
					InstanceID: "vm.ns", Hostname: "vm", UUID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5",
				},
			}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan).To(Equal(&cloudinit.IsoPlan{
				VolumeID: "config-2", FileName: "configdrive.iso", DataDir: "openstack/latest",
				Files: []cloudinit.IsoFile{
					{Path: "openstack/latest/user_data", Data: []byte("user")},
					{Path: "openstack/latest/meta_data.json", Data: []byte(`{"instance_id":"vm.ns","hostname":"vm","uuid":"cf73b4f4-6975-4d93-b51c-b7ae392bdea5"}`)},
					{Path: "openstack/latest/network_data.json", Data: []byte("network")},
				},
			}))
		})

		It("omits the network file when no network data is supplied", func() {
			data := &cloudinit.CloudInitData{
				DataSource: cloudinit.DataSourceConfigDrive,
				UserData:   "user",
				ConfigDriveMetaData: &cloudinit.ConfigDriveMetadata{
					InstanceID: "vm.ns", Hostname: "vm", UUID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5",
				},
			}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Files).To(Equal([]cloudinit.IsoFile{
				{Path: "openstack/latest/user_data", Data: []byte("user")},
				{Path: "openstack/latest/meta_data.json", Data: []byte(`{"instance_id":"vm.ns","hostname":"vm","uuid":"cf73b4f4-6975-4d93-b51c-b7ae392bdea5"}`)},
			}))
		})

		It("includes an empty user_data file for network-only input", func() {
			data := &cloudinit.CloudInitData{DataSource: cloudinit.DataSourceConfigDrive, NetworkData: "network", ConfigDriveMetaData: &cloudinit.ConfigDriveMetadata{InstanceID: "vm.ns"}}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(plan.Files[0]).To(Equal(cloudinit.IsoFile{Path: "openstack/latest/user_data", Data: []byte{}}))
			Expect(plan.Files[2]).To(Equal(cloudinit.IsoFile{Path: "openstack/latest/network_data.json", Data: []byte("network")}))
		})

		It("rejects generation without user or network data", func() {
			data := &cloudinit.CloudInitData{DataSource: cloudinit.DataSourceConfigDrive}
			plan, err := cloudinit.BuildIsoPlan(data)
			Expect(err).To(MatchError("UserData or NetworkData is required for cloud-init data source"))
			Expect(plan).NotTo(BeNil())
		})

	})
	It("serializes NoCloud metadata", func() {
		data := &cloudinit.CloudInitData{
			DataSource: cloudinit.DataSourceNoCloud,
			UserData:   "user",
			NoCloudMetaData: &cloudinit.NoCloudMetadata{
				InstanceID: "provided", InstanceType: "provided-type", LocalHostname: "host",
				PublicSSHKeys: map[string]string{"0": "key"},
			},
		}
		plan, err := cloudinit.BuildIsoPlan(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(plan.Files[1].Data)).To(MatchJSON(`{"instance-id":"provided","instance-type":"provided-type","local-hostname":"host","public-keys":{"0":"key"}}`))
	})

	It("serializes ConfigDrive metadata", func() {
		data := &cloudinit.CloudInitData{
			DataSource: cloudinit.DataSourceConfigDrive,
			UserData:   "user",
			ConfigDriveMetaData: &cloudinit.ConfigDriveMetadata{
				InstanceID: "provided", InstanceType: "provided-type",
				UUID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5", LocalHostname: "local", Hostname: "host",
				PublicSSHKeys: map[string]string{"0": "key"},
			},
		}
		plan, err := cloudinit.BuildIsoPlan(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(plan.Files[1].Data)).To(MatchJSON(`{"instance_type":"provided-type","instance_id":"provided","local_hostname":"local","hostname":"host","uuid":"cf73b4f4-6975-4d93-b51c-b7ae392bdea5","public_keys":{"0":"key"}}`))
	})

	It("includes ConfigDrive devices metadata", func() {
		devices := []cloudinit.DeviceData{
			{
				Type: cloudinit.HostDevMetadataType, Bus: "pci", Address: "0000:65:10:0",
				Serial: "serial", NumaNode: 1, AlignedCPUs: []uint32{0, 1}, Tags: []string{"tag"},
			},
			{
				Type: cloudinit.NICMetadataType, Bus: "pci", Address: "0000:01:00:0",
				MAC: "02:00:00:84:e9:58", Tags: []string{"tag"},
			},
		}
		data := &cloudinit.CloudInitData{
			DataSource:          cloudinit.DataSourceConfigDrive,
			UserData:            "user",
			ConfigDriveMetaData: &cloudinit.ConfigDriveMetadata{InstanceID: "provided"},
			DevicesData:         &devices,
		}
		plan, err := cloudinit.BuildIsoPlan(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(plan.Files[1].Data)).To(MatchJSON(`{
			"instance_id": "provided",
			"devices": [
				{"type":"hostdev","bus":"pci","address":"0000:65:10:0","serial":"serial","numaNode":1,"alignedCPUs":[0,1],"tags":["tag"]},
				{"type":"nic","bus":"pci","address":"0000:01:00:0","mac":"02:00:00:84:e9:58","tags":["tag"]}
			]
		}`))
	})

	It("rejects unsupported sources", func() {
		plan, err := cloudinit.BuildIsoPlan(&cloudinit.CloudInitData{DataSource: "unknown", UserData: "user"})
		Expect(err).To(MatchError("Invalid cloud-init data source: 'unknown'"))
		Expect(plan).To(BeNil())
		Expect(cloudinit.IsValidCloudInitData(nil)).To(BeFalse())
	})

})

func withAccessCredential(credential v1.AccessCredential) libvmi.Option {
	return func(vmi *v1.VirtualMachineInstance) {
		vmi.Spec.AccessCredentials = append(vmi.Spec.AccessCredentials, credential)
	}
}

func withNoCloudSSHPublicKey(secretName string) libvmi.Option {
	return withAccessCredential(v1.AccessCredential{
		SSHPublicKey: &v1.SSHPublicKeyAccessCredential{
			Source: v1.SSHPublicKeyAccessCredentialSource{
				Secret: &v1.AccessCredentialSecretSource{SecretName: secretName},
			},
			PropagationMethod: v1.SSHPublicKeyAccessCredentialPropagationMethod{
				NoCloud: &v1.NoCloudSSHPublicKeyAccessCredentialPropagation{},
			},
		},
	})
}

func withConfigDriveSSHPublicKey(secretName string) libvmi.Option {
	return withAccessCredential(v1.AccessCredential{
		SSHPublicKey: &v1.SSHPublicKeyAccessCredential{
			Source: v1.SSHPublicKeyAccessCredentialSource{
				Secret: &v1.AccessCredentialSecretSource{SecretName: secretName},
			},
			PropagationMethod: v1.SSHPublicKeyAccessCredentialPropagationMethod{
				ConfigDrive: &v1.ConfigDriveSSHPublicKeyAccessCredentialPropagation{},
			},
		},
	})
}
