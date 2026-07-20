/*
Copyright 2026.

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

package controller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	k8smaintenancev1alpha1 "github.com/k8s-operators-devops/app-maintenance-operator/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = k8smaintenancev1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs.
	// A clean checkout intentionally does not commit envtest binaries. Direct
	// `go test ./...` should still be useful, so skip only the envtest-backed
	// Ginkgo specs when assets are unavailable. `make test` provisions them.
	envtestBinaryDir := getFirstFoundEnvTestBinaryDir()
	if envtestBinaryDir != "" {
		testEnv.BinaryAssetsDirectory = envtestBinaryDir
	} else if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		Skip("envtest binaries are not installed; run make setup-envtest or make test")
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if cancel != nil {
		cancel()
	}
	if testEnv == nil || cfg == nil {
		return
	}
	err := testEnv.Stop()
	if err != nil && strings.Contains(err.Error(), "not supported by windows") {
		Expect(stopWindowsEnvTestProcesses()).To(Succeed())
		return
	}
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	var found string
	err := filepath.WalkDir(basePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() || found != "" {
			return nil
		}
		etcdName := "etcd"
		if os.PathSeparator == '\\' {
			etcdName = "etcd.exe"
		}
		etcdPath := filepath.Join(path, etcdName)
		if _, statErr := os.Stat(etcdPath); statErr == nil {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		logf.Log.Error(err, "Failed to scan envtest binary directory", "path", basePath)
		return ""
	}
	if found == "" {
		if _, err := os.Stat(basePath); err != nil {
			logf.Log.Error(err, "Failed to read directory", "path", basePath)
		}
	}
	return found
}

func stopWindowsEnvTestProcesses() error {
	if os.PathSeparator != '\\' {
		return nil
	}
	basePath, err := filepath.Abs(filepath.Join("..", "..", "bin", "k8s"))
	if err != nil {
		return err
	}
	script := `$base = "` + strings.ReplaceAll(basePath, "`", "``") + `"; ` +
		`Get-CimInstance Win32_Process | Where-Object { ` +
		`($_.Name -in @("etcd.exe","kube-apiserver.exe")) -and ` +
		`($_.ExecutablePath -like "$base*") ` +
		`} | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`
	return exec.Command("powershell", "-NoProfile", "-Command", script).Run()
}
