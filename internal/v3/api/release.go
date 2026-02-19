package v3

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dadav/gorge/internal/config"
	"github.com/dadav/gorge/internal/log"
	"github.com/dadav/gorge/internal/v3/backend"
	"github.com/dadav/gorge/internal/v3/utils"
	gen "github.com/dadav/gorge/pkg/gen/v3/openapi"
)

type ReleaseOperationsApi struct {
	gen.ReleaseOperationsAPIServicer
}

func NewReleaseOperationsApi() *ReleaseOperationsApi {
	return &ReleaseOperationsApi{}
}

type GetRelease404Response struct {
	Message string `json:"message,omitempty"`

	Errors []string `json:"errors,omitempty"`
}

// Add custom error types for better error handling
type ReleaseError struct {
	Code    int
	Message string
	Errors  []string
}

// AddRelease - Create module release
func (s *ReleaseOperationsApi) AddRelease(ctx context.Context, addReleaseRequest gen.AddReleaseRequest) (gen.ImplResponse, error) {
	if addReleaseRequest.File == "" {
		return gen.Response(400, gen.GetFile400Response{
			Message: "No file data provided",
			Errors:  []string{"file data is required"},
		}), nil
	}

	decodedTarball, err := base64.StdEncoding.DecodeString(addReleaseRequest.File)
	if err != nil {
		return gen.Response(400, gen.GetFile400Response{
			Message: "Invalid base64 encoded data",
			Errors:  []string{err.Error()},
		}), nil
	}

	release, err := backend.ConfiguredBackend.AddRelease(decodedTarball)
	if err != nil {
		return gen.Response(400, gen.GetFile400Response{
			Message: "Failed to add release",
			Errors:  []string{err.Error()},
		}), nil
	}

	return gen.Response(201, gen.ReleaseMinimal{
		Uri:     release.Uri,
		FileUri: release.FileUri,
		Slug:    release.Slug,
	}), nil
}

type DeleteRelease500Response struct {
	Message string   `json:"message,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// DeleteRelease - Delete module release
func (s *ReleaseOperationsApi) DeleteRelease(ctx context.Context, releaseSlug string, reason string) (gen.ImplResponse, error) {
	if !utils.CheckReleaseSlug(releaseSlug) {
		err := errors.New("invalid release slug")
		return gen.Response(
			400,
			DeleteRelease500Response{
				Message: err.Error(),
				Errors:  []string{err.Error()},
			},
		), nil
	}
	err := backend.ConfiguredBackend.DeleteReleaseBySlug(releaseSlug)
	if err == nil {
		return gen.Response(204, nil), nil
	}

	return gen.Response(
		500,
		DeleteRelease500Response{
			Message: err.Error(),
			Errors:  []string{err.Error()},
		},
	), nil
}

type GetFile400Response struct {
	Message string `json:"message,omitempty"`

	Errors []string `json:"errors,omitempty"`
}

// GetFile - Download module release
func (s *ReleaseOperationsApi) GetFile(ctx context.Context, filename string) (gen.ImplResponse, error) {
	if filename == "" {
		return gen.Response(400, gen.GetFile400Response{
			Message: "No filename provided",
			Errors:  []string{"filename is required"},
		}), nil
	}

	// Validate the filename to ensure it does not contain any path separators or parent directory references
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return gen.Response(400, gen.GetFile400Response{
			Message: "Invalid filename",
			Errors:  []string{"filename contains invalid characters"},
		}), nil
	}

	releaseSlug := strings.TrimSuffix(filename, ".tar.gz")
	if !utils.CheckReleaseSlug(releaseSlug) {
		return gen.Response(400, gen.GetFile400Response{
			Message: "Invalid release slug format",
			Errors:  []string{"release slug is invalid"},
		}), nil
	}

	release, err := backend.ConfiguredBackend.GetReleaseBySlug(releaseSlug)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return gen.Response(http.StatusNotFound, gen.GetFile404Response{
				Message: "File not found",
				Errors:  []string{"the file does not exist"},
			}), nil
		}
		return gen.Response(http.StatusInternalServerError, GetRelease500Response{
			Message: "Failed to resolve release",
			Errors:  []string{err.Error()},
		}), nil
	}

	filePath := filepath.Join(config.ModulesDir, release.Module.Slug, filename)

	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return gen.Response(http.StatusNotFound, gen.GetFile404Response{
				Message: "File not found",
				Errors:  []string{"the file does not exist"},
			}), nil
		}
		return gen.Response(http.StatusInternalServerError, GetRelease500Response{
			Message: "Failed to open file",
			Errors:  []string{err.Error()},
		}), nil
	}

	return gen.Response(http.StatusOK, f), nil
}

type GetRelease500Response struct {
	Message string   `json:"message,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// GetRelease - Fetch module release
func (s *ReleaseOperationsApi) GetRelease(ctx context.Context, releaseSlug string, withHtml bool, includeFields []string, excludeFields []string, ifModifiedSince string) (gen.ImplResponse, error) {
	release, err := backend.ConfiguredBackend.GetReleaseBySlug(releaseSlug)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return gen.Response(http.StatusNotFound, gen.GetFile404Response{
				Message: http.StatusText(http.StatusNotFound),
				Errors:  []string{"release not found"},
			}), nil
		}
		return gen.Response(http.StatusInternalServerError, GetRelease500Response{
			Message: http.StatusText(http.StatusInternalServerError),
			Errors:  []string{"error while reading release metadata"},
		}), nil
	}

	return gen.Response(http.StatusOK, release), nil
}

func abbrReleaseToFullReleasePlan(abbrReleasePlan gen.ReleasePlanAbbreviated) gen.ReleasePlan {
	planFile := fmt.Sprintf("plans/%s.pp", strings.Join(strings.Split(abbrReleasePlan.Name, "::")[1:], "/"))
	return gen.ReleasePlan{
		Uri:      abbrReleasePlan.Uri,
		Name:     abbrReleasePlan.Name,
		Private:  abbrReleasePlan.Private,
		Filename: planFile,
		PlanMetadata: gen.ReleasePlanPlanMetadata{
			Name:    abbrReleasePlan.Name,
			Private: abbrReleasePlan.Private,
			File:    planFile,
		},
	}
}

// GetReleasePlan - Fetch module release plan
func (s *ReleaseOperationsApi) GetReleasePlan(ctx context.Context, releaseSlug string, planName string) (gen.ImplResponse, error) {
	release, err := backend.ConfiguredBackend.GetReleaseBySlug(releaseSlug)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return gen.Response(http.StatusNotFound, gen.GetFile404Response{
				Message: http.StatusText(http.StatusNotFound),
				Errors:  []string{"plan not found"},
			}), nil
		}
		return gen.Response(http.StatusInternalServerError, GetRelease500Response{
			Message: http.StatusText(http.StatusInternalServerError),
			Errors:  []string{"error while reading release metadata"},
		}), nil
	}

	for _, plan := range release.Plans {
		if plan.Name == planName {
			// modulename::foo becomes plans/foo.pp
			return gen.Response(200, abbrReleaseToFullReleasePlan(plan)), nil
		}
	}
	return gen.Response(http.StatusNotFound, gen.GetFile404Response{
		Message: http.StatusText(http.StatusNotFound),
		Errors:  []string{"plan not found"},
	}), nil
}

// GetReleasePlans - List module release plans
func (s *ReleaseOperationsApi) GetReleasePlans(ctx context.Context, releaseSlug string) (gen.ImplResponse, error) {
	release, err := backend.ConfiguredBackend.GetReleaseBySlug(releaseSlug)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return gen.Response(http.StatusNotFound, gen.GetFile404Response{
				Message: http.StatusText(http.StatusNotFound),
				Errors:  []string{"plan not found"},
			}), nil
		}
		return gen.Response(http.StatusInternalServerError, GetRelease500Response{
			Message: http.StatusText(http.StatusInternalServerError),
			Errors:  []string{"error while reading release metadata"},
		}), nil
	}

	results := []gen.ReleasePlan{}
	for _, plan := range release.Plans {
		results = append(results, abbrReleaseToFullReleasePlan(plan))
	}

	return gen.Response(200, gen.GetReleasePlans200Response{
		Pagination: gen.GetReleasePlans200ResponsePagination{},
		Results:    results,
	}), nil
}

// GetReleases - List module releases
func (s *ReleaseOperationsApi) GetReleases(ctx context.Context, limit int32, offset int32, sortBy string, module string, owner string, withPdk bool, operatingsystem string, operatingsystemrelease string, peRequirement string, puppetRequirement string, moduleGroups []string, showDeleted bool, hideDeprecated bool, withHtml bool, includeFields []string, excludeFields []string, ifModifiedSince string, supported bool) (gen.ImplResponse, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if offset < 0 {
		offset = 0
	}

	results := []gen.Release{}
	filtered := []*gen.Release{}
	allReleases, _ := backend.ConfiguredBackend.GetAllReleases()

	base, _ := url.Parse("/v3/releases")
	params := url.Values{}

	filterSet := false

	if module != "" {
		filterSet = true
		params.Add("module", module)
	}

	if owner != "" {
		filterSet = true
		params.Add("owner", owner)
	}

	params.Add("offset", strconv.Itoa(int(offset)))
	params.Add("limit", strconv.Itoa(int(limit)))

	base.RawQuery = params.Encode()
	currentInf := interface{}(base.String())

	// We know there's no releases and a fallback proxy, so we should return a 404 to let the proxy handle it
	if config.FallbackProxyUrl != "" && len(allReleases) == 0 {
		log.Log.Debugln("Could not find *any* releases in the backend, returning 404 so we can proxy if desired")

		return gen.Response(http.StatusNotFound, GetRelease404Response{
			Message: "No releases found",
			Errors:  []string{"Did not retrieve any releases from the backend."},
		}), nil
	}

	if module != "" {
		// Perform an early query to see if the module even exists in the backend, optimization for instances with _many_ modules
		_, err := backend.ConfiguredBackend.GetModuleBySlug(module)
		if err != nil {
			log.Log.Debugf("Could not find module with slug '%s' in backend, returning 404 so we can proxy if desired\n", module)

			if config.FallbackProxyUrl != "" {
				return gen.Response(http.StatusNotFound, GetRelease404Response{
					Message: "No releases found",
					Errors:  []string{"No module(s) found for given query."},
				}), nil
			} else {
				return gen.Response(http.StatusOK, gen.GetReleases200Response{
					Pagination: gen.GetReleases200ResponsePagination{
						Limit:    limit,
						Offset:   offset,
						First:    &currentInf,
						Previous: nil,
						Current:  &currentInf,
						Next:     nil,
						Total:    0,
					},
					Results: []gen.Release{},
				}), nil
			}
		}
	}

	if filterSet {
		// We search through all available releases to see if they match the filter
		for _, r := range allReleases {
			if module != "" && r.Module.Slug != module {
				continue
			}

			if owner != "" && r.Module.Owner.Slug != owner {
				continue
			}

			filtered = append(filtered, r)
		}
	} else {
		filtered = allReleases
	}

	if len(filtered) > int(offset) {
		i := 1
		for _, release := range filtered[offset:] {
			if i > int(limit) {
				break
			}

			results = append(results, *release)
			i++
		}
	}

	// If we're using a fallback-proxy, we should return a 404 so the proxy can handle the request
	if config.FallbackProxyUrl != "" && len(results) == 0 {
		if module != "" {
			log.Log.Debugf("No releases for '%s' found in backend\n", module)
		} else {
			log.Log.Debugln("No releases found in backend")
		}

		return gen.Response(http.StatusNotFound, GetRelease404Response{
			Message: "No releases found",
			Errors:  []string{"No release(s) found for given query."},
		}), nil
	}

	base.RawQuery = params.Encode()
	currentInf = interface{}(base.String())
	params.Set("offset", "0")
	firstInf := interface{}(base.String())

	var nextInf interface{}
	nextOffset := int(offset) + len(results)
	if nextOffset < len(filtered) {
		params.Set("offset", strconv.Itoa(nextOffset))
		nextInf = interface{}(base.String())
	}

	var prevInf *string
	prevOffset := int(offset) - int(limit)
	if prevOffset >= 0 {
		prevStr := base.String()
		prevInf = &prevStr
	}

	return gen.Response(http.StatusOK, gen.GetReleases200Response{
		Pagination: gen.GetReleases200ResponsePagination{
			Limit:    limit,
			Offset:   offset,
			First:    &firstInf,
			Previous: prevInf,
			Current:  &currentInf,
			Next:     &nextInf,
			Total:    int32(len(filtered)),
		},
		Results: results,
	}), nil
}
