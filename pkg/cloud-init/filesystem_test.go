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

var _ = Describe("ReadSecretData", func() {
	It("reads a secret file through a symlink", func() {
		root := GinkgoT().TempDir()
		writeFixture(root, "keys/..version/key", "ssh-key")
		Expect(os.Symlink("..version", filepath.Join(root, "keys/..data"))).To(Succeed())
		Expect(os.Symlink("..data/key", filepath.Join(root, "keys/key"))).To(Succeed())

		secretData := cloudinit.ReadSecretData(root, cloudinit.SecretReadRequirements{Files: []string{"keys/key"}})

		Expect(secretData.Files).To(Equal(map[string]cloudinit.SecretFile{"keys/key": {Data: "ssh-key"}}))
		Expect(secretData.Directories).To(BeEmpty())
	})

	It("returns directory entries sorted by name", func() {
		root := GinkgoT().TempDir()
		writeFixture(root, "keys/key-b", "ssh-b")
		writeFixture(root, "keys/key-a", "ssh-a")

		secretData := cloudinit.ReadSecretData(root, cloudinit.SecretReadRequirements{Directories: []string{"keys"}})

		Expect(secretData.Directories["keys"]).To(Equal(cloudinit.SecretDirectory{Entries: []cloudinit.SecretEntry{
			{Name: "key-a", Data: "ssh-a"},
			{Name: "key-b", Data: "ssh-b"},
		}}))
	})

	It("records an error for a missing file", func() {
		root := GinkgoT().TempDir()

		secretData := cloudinit.ReadSecretData(root, cloudinit.SecretReadRequirements{Files: []string{"missing"}})

		Expect(secretData.Files).To(HaveKey("missing"))
		Expect(secretData.Files["missing"].Err).To(MatchError(os.ErrNotExist))
	})

	It("records an error for a missing directory", func() {
		root := GinkgoT().TempDir()

		secretData := cloudinit.ReadSecretData(root, cloudinit.SecretReadRequirements{Directories: []string{"missing"}})

		Expect(secretData.Directories).To(HaveKey("missing"))
		Expect(secretData.Directories["missing"].Err).To(MatchError(os.ErrNotExist))
	})

	It("records an error for a broken symlink", func() {
		root := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(root, "keys"), 0700)).To(Succeed())
		Expect(os.Symlink("absent", filepath.Join(root, "keys/broken"))).To(Succeed())

		secretData := cloudinit.ReadSecretData(root, cloudinit.SecretReadRequirements{Directories: []string{"keys"}})

		directory := secretData.Directories["keys"]
		Expect(directory.Err).NotTo(HaveOccurred())
		Expect(directory.Entries).To(HaveLen(1))
		Expect(directory.Entries[0].Name).To(Equal("broken"))
		Expect(directory.Entries[0].Err).To(MatchError(os.ErrNotExist))
	})

	It("records subdirectories without reading them as files", func() {
		root := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(root, "keys/subdir"), 0700)).To(Succeed())

		secretData := cloudinit.ReadSecretData(root, cloudinit.SecretReadRequirements{Directories: []string{"keys"}})

		Expect(secretData.Directories["keys"]).To(Equal(cloudinit.SecretDirectory{Entries: []cloudinit.SecretEntry{
			{Name: "subdir", IsDir: true},
		}}))
	})
})

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
			writeFixture(root, "cloudinitdisk/userdata", "secret-user")
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
			writeFixture(root, "cloudinitdisk/userdata", "secret-user")
			writeFixture(root, "cloudinitdisk/networkdata", "secret-network")

			data, err := cloudinit.ReadCloudInitVolumeDataSource(vmi, root)

			Expect(err).NotTo(HaveOccurred())
			Expect(data.DataSource).To(Equal(cloudinit.DataSourceConfigDrive))
			Expect(data.UserData).To(Equal("secret-user"))
			Expect(data.NetworkData).To(Equal("secret-network"))
		})
	})
})

var _ = Describe("Filesystem directory failures", func() {
	It("reports errors when a regular file blocks directory access", func() {
		root := GinkgoT().TempDir()
		writeFixture(root, "blocked", "regular file")
		secretData := cloudinit.ReadSecretData(root, cloudinit.SecretReadRequirements{Files: []string{"blocked/key"}, Directories: []string{"blocked"}})
		Expect(secretData.Files["blocked/key"].Err).To(HaveOccurred())
		Expect(secretData.Directories["blocked"].Err).To(HaveOccurred())

	})
})
