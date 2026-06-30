//go:build acceptance

package acceptance_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	gen "github.com/dadav/gorge/pkg/gen/v3/openapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Gorge API", func() {
	var url string

	BeforeEach(func() {
		url = baseURL(serverPort)
	})

	Describe("Health checks", func() {
		It("GET /readyz returns 200", func() {
			resp, body, err := doRequest("GET", url+"/readyz", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			Expect(body).To(MatchJSON(`{"message":"ok"}`))
		})

		It("GET /livez returns 200", func() {
			resp, body, err := doRequest("GET", url+"/livez", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			Expect(body).To(MatchJSON(`{"message":"ok"}`))
		})
	})

	Describe("Modules listing", func() {
		It("lists all modules with pagination", func() {
			resp, body, err := doRequest("GET", url+"/v3/modules", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetModules200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Pagination.Total).To(BeNumerically(">=", 2))
			Expect(len(result.Results)).To(BeNumerically(">=", 2))
			Expect(result.Pagination.Limit).To(Equal(int32(20)))
			Expect(result.Pagination.Offset).To(Equal(int32(0)))
		})

		It("respects limit parameter", func() {
			resp, body, err := doRequest("GET", url+"/v3/modules?limit=1", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetModules200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Results)).To(Equal(1))
			Expect(result.Pagination.Limit).To(Equal(int32(1)))
		})

		It("filters by owner", func() {
			resp, body, err := doRequest("GET", url+"/v3/modules?owner=puppetlabs", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetModules200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Results)).To(BeNumerically(">=", 2))
			for _, m := range result.Results {
				Expect(m.Owner.Username).To(Equal("puppetlabs"))
			}
		})

		It("filters by query", func() {
			resp, body, err := doRequest("GET", url+"/v3/modules?query=apache", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetModules200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			for _, m := range result.Results {
				Expect(m.Slug).To(ContainSubstring("apache"))
			}
		})

		It("returns empty results for non-matching query", func() {
			resp, body, err := doRequest("GET", url+"/v3/modules?query=nonexistent", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetModules200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Results)).To(Equal(0))
		})
	})

	Describe("Single module", func() {
		It("GET /v3/modules/{slug} returns the module", func() {
			resp, body, err := doRequest("GET", url+"/v3/modules/puppetlabs-apache", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.Module
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Slug).To(Equal("puppetlabs-apache"))
			Expect(result.Name).To(Equal("apache"))
			Expect(result.Owner.Username).To(Equal("puppetlabs"))
			Expect(result.CurrentRelease.Version).To(Equal("2.0.0"))
			Expect(result.Uri).To(Equal("/v3/modules/puppetlabs-apache"))
		})

		It("returns 404 for non-existent module", func() {
			resp, _, err := doRequest("GET", url+"/v3/modules/no-such-module", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})
	})

	Describe("Releases listing", func() {
		It("lists all releases with pagination", func() {
			resp, body, err := doRequest("GET", url+"/v3/releases", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetReleases200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Pagination.Total).To(BeNumerically(">=", 3))
			Expect(len(result.Results)).To(BeNumerically(">=", 3))
		})

		It("filters by module slug", func() {
			resp, body, err := doRequest("GET", url+"/v3/releases?module=puppetlabs-apache", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetReleases200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Pagination.Total).To(Equal(int32(2)))
			Expect(len(result.Results)).To(Equal(2))
			for _, r := range result.Results {
				Expect(r.Module.Slug).To(Equal("puppetlabs-apache"))
			}
		})

		It("filters by owner", func() {
			resp, body, err := doRequest("GET", url+"/v3/releases?owner=puppetlabs", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetReleases200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			for _, r := range result.Results {
				Expect(r.Module.Owner.Slug).To(Equal("puppetlabs"))
			}
		})
	})

	Describe("Single release", func() {
		It("GET /v3/releases/{slug} returns the release", func() {
			resp, body, err := doRequest("GET", url+"/v3/releases/puppetlabs-apache-1.0.0", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.Release
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Slug).To(Equal("puppetlabs-apache-1.0.0"))
			Expect(result.Version).To(Equal("1.0.0"))
			Expect(result.Module.Slug).To(Equal("puppetlabs-apache"))
			Expect(result.FileUri).To(Equal("/v3/files/puppetlabs-apache-1.0.0.tar.gz"))
			Expect(result.Module.Owner.Username).To(Equal("puppetlabs"))
		})

		It("returns 404 for non-existent release", func() {
			resp, _, err := doRequest("GET", url+"/v3/releases/nonexistent-module-9.9.9", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})
	})

	Describe("Release plans", func() {
		It("lists plans for a release (empty)", func() {
			resp, body, err := doRequest("GET", url+"/v3/releases/puppetlabs-apache-1.0.0/plans", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var result gen.GetReleasePlans200Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Results)).To(Equal(0))
		})

		It("returns 404 for non-existent plan", func() {
			resp, _, err := doRequest("GET", url+"/v3/releases/puppetlabs-apache-1.0.0/plans/nonexistent::plan", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})

		It("returns 404 for non-existent release plans", func() {
			resp, _, err := doRequest("GET", url+"/v3/releases/nonexistent-9.9.9/plans", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})
	})

	Describe("File download", func() {
		It("downloads a release tarball", func() {
			resp, body, err := doRequest("GET", url+"/v3/files/puppetlabs-apache-1.0.0.tar.gz", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			Expect(len(body)).To(BeNumerically(">", 0))
			Expect(resp.Header.Get("Content-Type")).NotTo(Equal("application/json"))
		})

		It("returns 404 for non-existent file", func() {
			resp, _, err := doRequest("GET", url+"/v3/files/puppetlabs-nonexistent-9.9.9.tar.gz", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})

		It("returns 400 for filename with path separator", func() {
			resp, _, err := doRequest("GET", url+"/v3/files/foo%2Fbar.tar.gz", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(400))
		})
	})

	Describe("Creating releases", func() {
		var tarballData []byte

		BeforeEach(func() {
			var err error
			tarballData, err = generateTarball(
				"testuser-createrelease", "1.0.0",
				"testuser", "MIT", "A test release for create tests", nil,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("POST /v3/releases creates a release from base64 tarball", func() {
			b64 := base64.StdEncoding.EncodeToString(tarballData)
			payload, _ := json.Marshal(gen.AddReleaseRequest{File: b64})
			resp, body, err := doRequest("POST", url+"/v3/releases", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(201))

			var result gen.ReleaseMinimal
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Slug).To(Equal("testuser-createrelease-1.0.0"))
			Expect(result.Uri).To(Equal("/v3/releases/testuser-createrelease-1.0.0"))
			Expect(result.FileUri).To(Equal("/v3/files/testuser-createrelease-1.0.0.tar.gz"))
		})

		It("returns 400 for empty file field", func() {
			payload, _ := json.Marshal(gen.AddReleaseRequest{File: ""})
			resp, body, err := doRequest("POST", url+"/v3/releases", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(400))

			var result gen.GetFile400Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Message).To(Equal("No file data provided"))
		})

		It("returns 400 for invalid base64", func() {
			payload, _ := json.Marshal(gen.AddReleaseRequest{File: "!!not-base64!!"})
			resp, body, err := doRequest("POST", url+"/v3/releases", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(400))

			var result gen.GetFile400Response
			err = json.Unmarshal(body, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Message).To(Equal("Invalid base64 encoded data"))
		})
	})

	Describe("Deleting releases", func() {
		It("DELETE /v3/releases/{slug} deletes a release", func() {
			resp, _, err := doRequest("DELETE", url+"/v3/releases/puppetlabs-stdlib-1.0.0?reason=test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(204))
		})

		It("returns 400 for invalid release slug format", func() {
			resp, _, err := doRequest("DELETE", url+"/v3/releases/bad-slug?reason=test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(400))
		})
	})

	Describe("Deprecating modules", func() {
		var deprecateURL string

		BeforeEach(func() {
			deprecateURL = url + "/v3/modules/testuser-createrelease"
		})

		It("PATCH /v3/modules/{slug} deprecates a module", func() {
			payload := `{"action":"deprecate","params":{"reason":"testing deprecation","replacement_slug":""}}`
			resp, _, err := doRequest("PATCH", deprecateURL, []byte(payload))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(204))
		})

		It("returns 404 for non-existent module", func() {
			payload := `{"action":"deprecate","params":{"reason":"test"}}`
			resp, _, err := doRequest("PATCH", url+"/v3/modules/test-nonexistent", []byte(payload))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})
	})

	Describe("Deleting modules", func() {
		It("DELETE /v3/modules/{slug} deletes a module", func() {
			resp, _, err := doRequest("DELETE", url+"/v3/modules/testuser-createrelease?reason=test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(204))
		})

		It("returns 400 for invalid module slug format", func() {
			resp, _, err := doRequest("DELETE", url+"/v3/modules/badslug?reason=test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(400))
		})
	})

	Describe("Not implemented endpoints", func() {
		It("GET /v3/users returns 501", func() {
			resp, _, err := doRequest("GET", url+"/v3/users", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(501))
		})

		It("GET /v3/users/{slug} returns 501", func() {
			resp, _, err := doRequest("GET", url+"/v3/users/testuser", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(501))
		})

		It("GET /v3/search_filters returns 501", func() {
			resp, _, err := doRequest("GET", url+"/v3/search_filters", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(501))
		})

		It("POST /v3/search_filters returns 501", func() {
			resp, _, err := doRequest("POST", url+"/v3/search_filters?search_filter_slug=test", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(501))
		})

		It("DELETE /v3/search_filters/{id} returns 501", func() {
			resp, _, err := doRequest("DELETE", url+"/v3/search_filters/1", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(501))
		})
	})

	Describe("Proxy and import from real forge", Label("network"), func() {
		BeforeEach(func() {
			if os.Getenv("GORGE_NETWORK_TESTS") != "1" {
				Skip("Set GORGE_NETWORK_TESTS=1 to run network-dependent tests")
			}
		})

		It("downloads and imports a module from forgeapi.puppet.com", func() {
			proxyModulesDir, err := os.MkdirTemp("", "gorge-proxy-modules-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(proxyModulesDir) }()

			proxyPort, err := findFreePort()
			Expect(err).NotTo(HaveOccurred())

			proxyCmd, err := startServer(binaryPath, proxyModulesDir, proxyPort,
				"--fallback-proxy", "https://forgeapi.puppet.com",
				"--import-proxied-releases",
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = proxyCmd.Process.Signal(os.Interrupt)
				_ = proxyCmd.Wait()
			}()

			err = waitForReady(fmt.Sprintf("%d", proxyPort), 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			proxyURL := baseURL(proxyPort)

			// Request a known module file from the real forge
			resp, body, err := doRequest("GET", proxyURL+"/v3/files/puppetlabs-stdlib-6.0.0.tar.gz", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			Expect(len(body)).To(BeNumerically(">", 100))
			Expect(resp.Header.Get("X-Proxied-To")).To(Equal("https://forgeapi.puppet.com"))

			// Verify the module was imported and is now served locally
			resp2, body2, err := doRequest("GET", proxyURL+"/v3/modules/puppetlabs-stdlib", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp2.StatusCode).To(Equal(200))

			var module gen.Module
			err = json.Unmarshal(body2, &module)
			Expect(err).NotTo(HaveOccurred())
			Expect(module.Slug).To(Equal("puppetlabs-stdlib"))
			Expect(module.CurrentRelease.Version).To(Equal("6.0.0"))

			// A second request confirms the file is still accessible (may be cache or local)
			resp3, _, err := doRequest("GET", proxyURL+"/v3/files/puppetlabs-stdlib-6.0.0.tar.gz", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp3.StatusCode).To(Equal(200))
		})
	})
})
