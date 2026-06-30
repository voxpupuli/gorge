//go:build acceptance

package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	serverCmd   *exec.Cmd
	serverPort  int
	binaryPath  string
	modulesDir  string
	binaryBuilt bool
)

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gorge Acceptance Suite")
}

var _ = BeforeSuite(func() {
	var err error

	binaryPath = filepath.Join(os.TempDir(), "gorge-acceptance-test")
	err = buildBinary(binaryPath)
	Expect(err).NotTo(HaveOccurred(), "should build the gorge binary")
	binaryBuilt = true

	modulesDir, err = os.MkdirTemp("", "gorge-modules-*")
	Expect(err).NotTo(HaveOccurred(), "should create temp modules dir")

	_, err = writeTarball(modulesDir, "puppetlabs-apache", "1.0.0", "puppetlabs", "Apache-2.0", "A test module for apache", nil)
	Expect(err).NotTo(HaveOccurred())

	_, err = writeTarball(modulesDir, "puppetlabs-apache", "2.0.0", "puppetlabs", "Apache-2.0", "A test module for apache", nil)
	Expect(err).NotTo(HaveOccurred())

	_, err = writeTarball(modulesDir, "puppetlabs-stdlib", "1.0.0", "puppetlabs", "Apache-2.0", "Standard library", []string{"stdlib", "puppetlabs"})
	Expect(err).NotTo(HaveOccurred())

	serverPort, err = findFreePort()
	Expect(err).NotTo(HaveOccurred(), "should find a free port")

	serverCmd, err = startServer(binaryPath, modulesDir, serverPort)
	Expect(err).NotTo(HaveOccurred(), "should start the gorge server")

	err = waitForReady(fmt.Sprintf("%d", serverPort), 10*time.Second)
	Expect(err).NotTo(HaveOccurred(), "server should become ready within 10 seconds")
})

var _ = AfterSuite(func() {
	if serverCmd != nil && serverCmd.Process != nil {
		_ = serverCmd.Process.Signal(os.Interrupt)
		_ = serverCmd.Wait()
	}

	if binaryBuilt {
		_ = os.Remove(binaryPath)
	}

	if modulesDir != "" {
		_ = os.RemoveAll(modulesDir)
	}
})
