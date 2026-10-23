/*
 * This file is part of the kubevirt project
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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cloudinit "kubevirt.io/kubevirt/pkg/cloud-init"
	"kubevirt.io/kubevirt/pkg/libvmi"
	cloudinitdata "kubevirt.io/kubevirt/pkg/libvmi/cloudinit"
)

func writeFixture(root, name, data string) {
	GinkgoHelper()
	path := filepath.Join(root, name)
	Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
	Expect(os.WriteFile(path, []byte(data), 0600)).To(Succeed())
}

func symlinkFixture(root, name, target string) {
	GinkgoHelper()
	path := filepath.Join(root, name)
	Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
	Expect(os.Symlink(target, path)).To(Succeed())
}

var _ = Describe("ReadCloudInitVolumeDataSource", func() {
	Context("NoCloud", func() {
		It("reads user and network data from mounted secrets", func() {
			root := GinkgoT().TempDir()
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitNoCloud(
					cloudinitdata.WithNoCloudUserDataSecretName("user"),
					cloudinitdata.WithNoCloudNetworkDataSecretName("network"),
				),
			)
			writeFixture(root, "cloudinitdisk/..version/userdata", "secret-user")
			symlinkFixture(root, "cloudinitdisk/..data", "..version")
			symlinkFixture(root, "cloudinitdisk/userdata", "..data/userdata")
			writeFixture(root, "cloudinitdisk/networkdata", "secret-network")

			data, err := cloudinit.ReadCloudInitVolumeDataSource(vmi, root)

			Expect(err).NotTo(HaveOccurred())
			Expect(data.DataSource).To(Equal(cloudinit.DataSourceNoCloud))
			Expect(data.UserData).To(Equal("secret-user"))
			Expect(data.NetworkData).To(Equal("secret-network"))
		})
	})

	Context("ConfigDrive", func() {
		It("reads user and network data from mounted secrets", func() {
			root := GinkgoT().TempDir()
			vmi := libvmi.New(
				libvmi.WithFirmwareUUID("cf73b4f4-6975-4d93-b51c-b7ae392bdea5"),
				libvmi.WithCloudInitConfigDrive(
					cloudinitdata.WithConfigDriveUserDataSecretName("user"),
					cloudinitdata.WithConfigDriveNetworkDataSecretName("network"),
				),
			)
			writeFixture(root, "cloudinitdisk/..version/userdata", "secret-user")
			symlinkFixture(root, "cloudinitdisk/..data", "..version")
			symlinkFixture(root, "cloudinitdisk/userdata", "..data/userdata")
			writeFixture(root, "cloudinitdisk/networkdata", "secret-network")

			data, err := cloudinit.ReadCloudInitVolumeDataSource(vmi, root)

			Expect(err).NotTo(HaveOccurred())
			Expect(data.DataSource).To(Equal(cloudinit.DataSourceConfigDrive))
			Expect(data.UserData).To(Equal("secret-user"))
			Expect(data.NetworkData).To(Equal("secret-network"))
		})
	})
})

var _ = Describe("GenerateIsoInDir", func() {
	It("preserves the published ISO and cleans staging and input files when generation fails", func() {
		cache := GinkgoT().TempDir()
		root := filepath.Join(cache, "default", "guest")
		vmi := libvmi.New(libvmi.WithName("guest"), libvmi.WithNamespace("default"))
		data := &cloudinit.CloudInitData{
			DataSource: cloudinit.DataSourceNoCloud,
			UserData:   "user",
		}
		writeFixture(root, "noCloud.iso", "existing ISO")
		writeFixture(root, "noCloud.iso.staging", "stale staging file")
		GinkgoT().Setenv("PATH", GinkgoT().TempDir())

		Expect(cloudinit.GenerateIsoInDir(cache, vmi, data)).NotTo(Succeed())

		content, err := os.ReadFile(filepath.Join(root, "noCloud.iso"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("existing ISO"))
		entries, err := os.ReadDir(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name()).To(Equal("noCloud.iso"))
	})
})
