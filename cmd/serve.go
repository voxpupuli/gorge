/*
Copyright © 2024 dadav

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
package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	config "github.com/dadav/gorge/internal/config"
	log "github.com/dadav/gorge/internal/log"
	customMiddleware "github.com/dadav/gorge/internal/middleware"
	"github.com/dadav/gorge/internal/utils"
	v3 "github.com/dadav/gorge/internal/v3/api"
	backend "github.com/dadav/gorge/internal/v3/backend"
	"github.com/dadav/gorge/internal/v3/ui"
	openapi "github.com/dadav/gorge/pkg/gen/v3/openapi"
	"github.com/dadav/stampede"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the puppet forge webserver",
	Long: `Run this command to start serving your own puppet modules.
You can also enable fallback proxies to forward the requests to
when you don't have the requested module in your local module
set yet.

You can also enable the caching functionality to speed things up.`,
	Run: func(_ *cobra.Command, _ []string) {
		var err error

		log.Setup(config.Dev, config.Debug)

		config.ModulesDir, err = utils.ExpandTilde(config.ModulesDir)
		if err != nil {
			log.Log.Fatal(err)
		}
		config.TlsCertPath, err = utils.ExpandTilde(config.TlsCertPath)
		if err != nil {
			log.Log.Fatal(err)
		}
		config.TlsKeyPath, err = utils.ExpandTilde(config.TlsKeyPath)
		if err != nil {
			log.Log.Fatal(err)
		}
		config.JwtTokenPath, err = utils.ExpandTilde(config.JwtTokenPath)
		if err != nil {
			log.Log.Fatal(err)
		}

		if config.Backend == "filesystem" {
			backend.ConfiguredBackend = backend.NewFilesystemBackend(config.ModulesDir)
		} else {
			log.Log.Fatalf("Invalid backend: %s", config.Backend)
		}

		if _, err := os.Stat(config.ModulesDir); err != nil {
			err = os.MkdirAll(config.ModulesDir, os.ModePerm)
			if err != nil {
				log.Log.Fatal(err)
			}
			if config.DropPrivileges && utils.IsRoot() {
				uid, err := strconv.Atoi(config.User)
				if err != nil {
					u, err := user.Lookup(config.User)
					if err != nil {
						log.Log.Fatal(err)
					}
					uid, err = strconv.Atoi(u.Uid)
					if err != nil {
						log.Log.Fatal(err)
					}
				}
				gid, err := strconv.Atoi(config.Group)
				if err != nil {
					g, err := user.LookupGroup(config.Group)
					if err != nil {
						log.Log.Fatal(err)
					}
					gid, err = strconv.Atoi(g.Gid)
					if err != nil {
						log.Log.Fatal(err)
					}
				}
				err = os.Chown(config.ModulesDir, uid, gid)
				if err != nil {
					log.Log.Fatal(err)
				}
			}
		}

		if config.ApiVersion == "v3" {
			moduleService := v3.NewModuleOperationsApi()
			releaseService := v3.NewReleaseOperationsApi()
			searchFilterService := v3.NewSearchFilterOperationsApi()
			userService := v3.NewUserOperationsApi()

			r := chi.NewRouter()

			// 0. Inject statistic middleware
			x := customMiddleware.NewStatistics()
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctx := context.WithValue(r.Context(), customMiddleware.StatsKey, x)
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			})

			// 1. Recoverer should be first to catch panics in all other middleware
			r.Use(middleware.Recoverer)
			// 2. RealIP should be early to ensure all other middleware sees the correct IP
			r.Use(middleware.RealIP)
			// 3. CORS should be early as it might reject requests before doing unnecessary work
			r.Use(cors.Handler(cors.Options{
				AllowedOrigins:   strings.Split(config.CORSOrigins, ","),
				AllowedMethods:   []string{"GET", "POST", "DELETE", "PATCH"},
				AllowedHeaders:   []string{"Accept", "Content-Type"},
				AllowCredentials: false,
				MaxAge:           300,
			}))
			// 4. RequireUserAgent should be early to ensure all other middleware sees the correct user agent
			r.Use(customMiddleware.RequireUserAgent)

			if config.UI {
				r.Group(func(r chi.Router) {
					r.HandleFunc("/", ui.IndexHandler)
					r.HandleFunc("/search", ui.SearchHandler)
					r.HandleFunc("/modules/{module}", ui.ModuleHandler)
					r.HandleFunc("/modules/{module}/{version}", ui.ReleaseHandler)
					r.HandleFunc("/authors/{author}", ui.AuthorHandler)
					r.HandleFunc("/statistics", ui.StatisticsHandler(x))
					r.Handle("/assets/*", ui.HandleAssets())
				})
			}

			r.Group(func(r chi.Router) {
				if !config.NoCache {
					log.Log.Debug("Setting up cache middleware")
					customKeyFunc := func(r *http.Request) uint64 {
						token := r.Header.Get("Authorization")
						requestURI := r.URL.Path

						if config.CacheByFullRequestURI {
							requestURI = r.URL.RequestURI()
						}

						return stampede.StringToHash(r.Method, requestURI, strings.ToLower(token))
					}

					cbFunc := func(fromCache bool, w http.ResponseWriter, r *http.Request) error {
						x.Mutex.Lock()
						if fromCache {
							log.Log.Debugf("Cache hit for path: %s", r.URL.Path)
							x.TotalCacheHits++
							x.CacheHitsPerEndpoint[r.URL.Path]++
							w.Header().Set("X-Cache", "Hit from gorge")
						} else {
							log.Log.Debugf("Cache miss for path: %s", r.URL.Path)
							x.TotalCacheMisses++
							x.CacheMissesPerEndpoint[r.URL.Path]++
							w.Header().Set("X-Cache", "MISS from gorge")
						}
						x.Mutex.Unlock()
						return nil
					}

					cachedMiddleware := stampede.HandlerWithKeyAndCb(
						512,
						time.Duration(config.CacheMaxAge)*time.Second,
						customKeyFunc,
						cbFunc,
					)
					log.Log.Debugf("Cache middleware configured with prefixes: %s", config.CachePrefixes)

					cachePrefixes := strings.Split(config.CachePrefixes, ",")

					r.Use(func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							shouldCache := false
							for _, prefix := range cachePrefixes {
								if strings.HasPrefix(r.URL.Path, strings.TrimSpace(prefix)) {
									shouldCache = true
									break
								}
							}

							if shouldCache {
								cachedMiddleware(next).ServeHTTP(w, r)
							} else {
								next.ServeHTTP(w, r)
							}
						})
					})
				}

				if config.FallbackProxyUrl != "" {
					proxies := strings.Split(config.FallbackProxyUrl, ",")
					slices.Reverse(proxies)

					proxyPrefixes := strings.Split(config.ProxyPrefixes, ",")

					for _, proxy := range proxies {
						r.Use(customMiddleware.ProxyFallback(
							proxy,
							func(r *http.Request, status int) bool {
								shouldProxy := false
								for _, prefix := range proxyPrefixes {
									if strings.HasPrefix(r.URL.Path, strings.TrimSpace(prefix)) {
										shouldProxy = true
										break
									}
								}
								return shouldProxy && status == http.StatusNotFound
							},
							func(r *http.Response) {
								r.Header.Add("X-Proxied-To", proxy)

								if config.ImportProxiedReleases && strings.HasPrefix(r.Request.URL.Path, "/v3/files/") && r.StatusCode == http.StatusOK {
									body, err := io.ReadAll(r.Body)
									if err != nil {
										log.Log.Error(err)
										return
									}

									// restore the body
									r.Body = io.NopCloser(bytes.NewBuffer(body))

									release, err := backend.ConfiguredBackend.AddRelease(body)
									if err != nil {
										log.Log.Error(err)
										return
									}

									log.Log.Infof("Imported release %s\n", release.Slug)
								}
							},
						))
					}
				}

				// StatisticsMiddleware should be last to ensure all other middleware is counted
				r.Use(customMiddleware.StatisticsMiddleware(x))

				apiRouter := openapi.NewRouter(
					openapi.NewModuleOperationsAPIController(moduleService),
					openapi.NewReleaseOperationsAPIController(releaseService),
					openapi.NewSearchFilterOperationsAPIController(searchFilterService),
					openapi.NewUserOperationsAPIController(userService),
				)

				r.Mount("/", apiRouter)
			})

			r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				_, _ = w.Write([]byte(`{"message": "ok"}`))
			})

			r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				_, _ = w.Write([]byte(`{"message": "ok"}`))
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Create signal handling context
			sigCtx, restoreDefaultSignalHandling := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
			defer restoreDefaultSignalHandling()
			g, gCtx := errgroup.WithContext(sigCtx)

			if err := backend.ConfiguredBackend.LoadModules(); err != nil {
				log.Log.Fatal(fmt.Errorf("initial module load failed: %w", err))
			}

			if config.ModulesScanSec > 0 {
				g.Go(func() error {
					ticker := time.NewTicker(time.Duration(config.ModulesScanSec) * time.Second)
					defer ticker.Stop()

					for {
						select {
						case <-gCtx.Done():
							return nil
						case <-ticker.C:
							if err := backend.ConfiguredBackend.LoadModules(); err != nil {
								log.Log.Errorf("Failed to load modules: %v", err)
								// Continue running instead of failing completely
							}
						}
					}
				})
			}

			bindPort := fmt.Sprintf("%s:%d", config.Bind, config.Port)
			listener, err := net.Listen("tcp", bindPort)
			if err != nil {
				log.Log.Fatal(err)
			}
			log.Log.Infof("Listen on %s", bindPort)

			server := http.Server{Handler: r, BaseContext: func(_ net.Listener) context.Context { return ctx }}
			wantTLS := config.TlsKeyPath != "" && config.TlsCertPath != ""

			if wantTLS {
				cert, err := tls.LoadX509KeyPair(config.TlsCertPath, config.TlsKeyPath)
				if err != nil {
					log.Log.Fatalf("Failed to load TLS certificates: %v", err)
				}

				tlsConfig := &tls.Config{
					Certificates: []tls.Certificate{cert},
					MinVersion:   tls.VersionTLS12,
				}
				server.TLSConfig = tlsConfig
			}

			if config.DropPrivileges && utils.IsRoot() {
				log.Log.Infof("Give control to user %s and group %s", config.User, config.Group)
				if err = utils.DropPrivileges(config.User, config.Group); err != nil {
					log.Log.Fatal(err)
				}
			}

			g.Go(func() error {
				if wantTLS {
					if err := server.ServeTLS(listener, "", ""); err != http.ErrServerClosed {
						return err
					}
				} else {
					if err := server.Serve(listener); err != http.ErrServerClosed {
						return err
					}
				}
				return nil
			})

			g.Go(func() error {
				<-gCtx.Done()
				shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancelShutdown()

				log.Log.Info("Shutting down server...")
				if err := server.Shutdown(shutdownCtx); err != nil {
					return fmt.Errorf("server shutdown failed: %w", err)
				}
				return nil
			})

			if err := g.Wait(); err != nil {
				log.Log.Panic(err)
			}
		} else {
			log.Log.Panicf("%s version not supported", config.ApiVersion)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVar(&config.User, "user", "", "give control to this user or uid (requires root)")
	serveCmd.Flags().StringVar(&config.Group, "group", "", "give control to this group or gid (requires root)")
	serveCmd.Flags().StringVar(&config.ApiVersion, "api-version", "v3", "the forge api version to use")
	serveCmd.Flags().IntVar(&config.Port, "port", 8080, "the port to listen to")
	serveCmd.Flags().StringVar(&config.Bind, "bind", "127.0.0.1", "host to listen to")
	serveCmd.Flags().StringVar(&config.ModulesDir, "modulesdir", "~/.gorge/modules", "directory containing all the modules")
	serveCmd.Flags().IntVar(&config.ModulesScanSec, "modules-scan-sec", 0, "seconds between scans of directory containing all the modules. (default 0 means only scan at startup)")
	serveCmd.Flags().StringVar(&config.Backend, "backend", "filesystem", "backend to use")
	serveCmd.Flags().StringVar(&config.CORSOrigins, "cors", "*", "allowed cors origins separated by comma")
	serveCmd.Flags().StringVar(&config.FallbackProxyUrl, "fallback-proxy", "", "optional comma separated list of fallback upstream proxy urls")
	serveCmd.Flags().BoolVar(&config.Dev, "dev", false, "enables dev mode")
	serveCmd.Flags().BoolVar(&config.Debug, "debug", false, "enables debug mode")
	serveCmd.Flags().BoolVar(&config.DropPrivileges, "drop-privileges", false, "drops privileges to the given user/group")
	serveCmd.Flags().BoolVar(&config.UI, "ui", false, "enables the web ui")
	serveCmd.Flags().StringVar(&config.CachePrefixes, "cache-prefixes", "/v3/files", "url prefixes to cache")
	serveCmd.Flags().StringVar(&config.ProxyPrefixes, "proxy-prefixes", "/v3", "url prefixes to proxy")
	serveCmd.Flags().StringVar(&config.JwtSecret, "jwt-secret", "changeme", "jwt secret")
	serveCmd.Flags().StringVar(&config.JwtTokenPath, "jwt-token-path", "~/.gorge/token", "jwt token path")
	serveCmd.Flags().StringVar(&config.TlsCertPath, "tls-cert", "", "path to tls cert file")
	serveCmd.Flags().StringVar(&config.TlsKeyPath, "tls-key", "", "path to tls key file")
	serveCmd.Flags().Int64Var(&config.CacheMaxAge, "cache-max-age", 86400, "max number of seconds responses should be cached")
	serveCmd.Flags().BoolVar(&config.NoCache, "no-cache", false, "disables the caching functionality")
	serveCmd.Flags().BoolVar(&config.ImportProxiedReleases, "import-proxied-releases", false, "add every proxied modules to local store")
	serveCmd.Flags().BoolVar(&config.CacheByFullRequestURI, "cache-by-full-request-uri", false, "will cache responses by the full request URI (incl. query fragments) instead of only the request path")
}
