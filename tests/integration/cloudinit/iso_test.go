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
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"kubevirt.io/client-go/testutils"

	cloudinit "kubevirt.io/kubevirt/pkg/cloud-init"
	"kubevirt.io/kubevirt/pkg/libvmi"
)

// These integration tests exercise real ISO creation and file ownership.
// Run with go test ./tests/integration/cloudinit, or with the Bazel target
// //tests/integration/cloudinit:go_default_test in a prepared builder container.
// It requires xorrisofs on PATH, a qemu account, and permission to
// chown files to that account (root or CAP_CHOWN). These prerequisites are
// provided by the KubeVirt builder image.
func TestCloudInitIsoIntegration(t *testing.T) {
	testutils.KubeVirtTestSuiteSetup(t)
}

var _ = Describe("Cloud-init ISO generation", func() {
	Context("NoCloud", func() {
		It("generates an aligned ISO with qemu ownership and removes temporary files", func() {
			cache := GinkgoT().TempDir()
			root := filepath.Join(cache, "default", "guest")
			vmi := libvmi.New(libvmi.WithName("guest"), libvmi.WithNamespace("default"))
			data := &cloudinit.CloudInitData{
				DataSource:      cloudinit.DataSourceNoCloud,
				UserData:        "#cloud-config\nuser: integration\n",
				NetworkData:     "network: integration\n",
				NoCloudMetaData: &cloudinit.NoCloudMetadata{InstanceID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5"},
			}

			Expect(cloudinit.GenerateIsoInDir(cache, vmi, data)).To(Succeed())

			expectQemuOwner(filepath.Join(root, "noCloud.iso"))
			expectIsoAlignment(filepath.Join(root, "noCloud.iso"))
			expectNoLeftovers(root, "noCloud.iso")
		})

	})

	Context("ConfigDrive", func() {
		It("generates an aligned ISO with qemu ownership and removes temporary files", func() {
			cache := GinkgoT().TempDir()
			root := filepath.Join(cache, "default", "guest")
			vmi := libvmi.New(libvmi.WithName("guest"), libvmi.WithNamespace("default"))
			data := &cloudinit.CloudInitData{
				DataSource:          cloudinit.DataSourceConfigDrive,
				UserData:            "#cloud-config\nuser: integration\n",
				NetworkData:         "network: integration\n",
				ConfigDriveMetaData: &cloudinit.ConfigDriveMetadata{InstanceID: "guest.default", UUID: "cf73b4f4-6975-4d93-b51c-b7ae392bdea5"},
			}

			Expect(cloudinit.GenerateIsoInDir(cache, vmi, data)).To(Succeed())

			expectQemuOwner(filepath.Join(root, "configdrive.iso"))
			expectIsoAlignment(filepath.Join(root, "configdrive.iso"))
			expectNoLeftovers(root, "configdrive.iso")
		})
	})
})

func writeFixture(root, name, data string) {
	GinkgoHelper()
	path := filepath.Join(root, name)
	Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
	Expect(os.WriteFile(path, []byte(data), 0600)).To(Succeed())
}

func expectQemuOwner(path string) {
	GinkgoHelper()
	owner, err := user.Lookup("qemu")
	Expect(err).NotTo(HaveOccurred())
	uid, err := strconv.Atoi(owner.Uid)
	Expect(err).NotTo(HaveOccurred())
	gid, err := strconv.Atoi(owner.Gid)
	Expect(err).NotTo(HaveOccurred())
	info, err := os.Stat(path)
	Expect(err).NotTo(HaveOccurred())
	stat, ok := info.Sys().(*syscall.Stat_t)
	Expect(ok).To(BeTrue())
	Expect(int(stat.Uid)).To(Equal(uid))
	Expect(int(stat.Gid)).To(Equal(gid))
}

func expectIsoAlignment(path string) {
	GinkgoHelper()
	info, err := os.Stat(path)
	Expect(err).NotTo(HaveOccurred())
	// Keep generated images compatible with node storage using 4 KiB blocks.
	Expect(info.Size()).To(BeNumerically(">", 0))
	Expect(info.Size()%4096).To(BeZero(), "ISO size must be a multiple of 4096 bytes")
}

func expectNoLeftovers(root, fileName string) {
	GinkgoHelper()
	inputs, err := filepath.Glob(filepath.Join(root, "cloud-init-data-*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(inputs).To(BeEmpty())

	_, err = os.Stat(filepath.Join(root, fileName+".staging"))
	Expect(err).To(MatchError(os.ErrNotExist))
}
