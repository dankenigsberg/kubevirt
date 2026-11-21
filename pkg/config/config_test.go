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
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/kubevirt/pkg/config"
)

var _ = Describe("Config ISO planning and publication", func() {
	It("preserves the custom volume label and grafts projected volume symlinks", func() {
		source := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(source, "..data"), 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(source, "..data", "key"), []byte("value"), 0600)).To(Succeed())
		Expect(os.Symlink("..data/key", filepath.Join(source, "key"))).To(Succeed())

		entries, err := os.ReadDir(source)
		Expect(err).NotTo(HaveOccurred())
		layout := config.BuildISOLayout("custom", source, entries)
		Expect(layout.Label).To(Equal("custom"))
		Expect(layout.GraftPoints).To(Equal([]string{
			"..data=" + filepath.Join(source, "..data"),
			"key=" + filepath.Join(source, "key"),
		}))
	})

})

func requireXorrisofs() {
	if _, err := exec.LookPath("xorrisofs"); err != nil {
		Skip("xorrisofs is required to write an ISO")
	}
}

func expectISO(path, label string) {
	image, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	const descriptor = 16 * 2048
	Expect(len(image)).To(BeNumerically(">=", descriptor+72))
	Expect(string(image[descriptor+1 : descriptor+6])).To(Equal("CD001"))
	Expect(strings.TrimSpace(string(image[descriptor+40 : descriptor+72]))).To(Equal(label))
}
