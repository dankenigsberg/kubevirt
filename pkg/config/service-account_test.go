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

package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/config"
	"kubevirt.io/kubevirt/pkg/libvmi"
)

var _ = Describe("ServiceAccount", func() {

	BeforeEach(func() {
		var err error

		config.ServiceAccountSourceDir, err = os.MkdirTemp("", "serviceaccount")
		Expect(err).NotTo(HaveOccurred())
		os.MkdirAll(config.ServiceAccountSourceDir, 0755)
		os.OpenFile(filepath.Join(config.ServiceAccountSourceDir, "token"), os.O_RDONLY|os.O_CREATE, 0666)
		os.OpenFile(filepath.Join(config.ServiceAccountSourceDir, "namespace"), os.O_RDONLY|os.O_CREATE, 0666)

		config.ServiceAccountDiskDir, err = os.MkdirTemp("", "serviceaccount-disk")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(config.ServiceAccountSourceDir)
		os.RemoveAll(config.ServiceAccountDiskDir)
	})

	It("Should create a new service account iso disk", func() {
		vmi := libvmi.New(
			libvmi.WithServiceAccountDisk("testaccount"),
		)

		requireXorrisofs()
		Expect(config.CreateServiceAccountDisk(vmi, false)).To(Succeed())
		expectISO(config.GetServiceAccountDiskPath(), "cfgdata")
	})

	It("Should not create a new service account iso disk without a Disk device", func() {
		vmi := libvmi.New()
		vmi.Spec.Volumes = append(vmi.Spec.Volumes, v1.Volume{
			Name: "serviceaccount-volume",
			VolumeSource: v1.VolumeSource{
				ServiceAccount: &v1.ServiceAccountVolumeSource{
					ServiceAccountName: "testaccount",
				},
			},
		})

		err := config.CreateServiceAccountDisk(vmi, false)
		Expect(err).NotTo(HaveOccurred())
		files, _ := os.ReadDir(config.ServiceAccountDiskDir)
		Expect(files).To(BeEmpty())
	})
})
