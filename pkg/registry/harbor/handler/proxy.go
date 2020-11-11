/*
 * Tencent is pleased to support the open source community by making TKEStack
 * available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */

package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"
	registryinternalclient "tkestack.io/tke/api/client/clientset/internalversion/typed/registry/internalversion"

	"tkestack.io/tke/api/registry"

	registryconfig "tkestack.io/tke/pkg/registry/apis/config"
	harbor "tkestack.io/tke/pkg/registry/harbor/client"
	"tkestack.io/tke/pkg/util/log"
	"tkestack.io/tke/pkg/util/transport"
)

type handler struct {
	reverseProxy   *httputil.ReverseProxy
	host           string
	externalHost   string
	manifestRegexp *regexp.Regexp
}

type HarborContextKey string

const manifestPattern = "/v2/.*/.*/manifests/.*"

var registryClient *registryinternalclient.RegistryClient
var harborClient *harbor.APIClient

// NewHandler to create a reverse proxy handler and returns it.
func NewHandler(address string, cafile string, externalHost string, loopbackConfig *restclient.Config, registryConfig *registryconfig.RegistryConfiguration) (http.Handler, error) {
	u, err := url.Parse(address)
	if err != nil {
		log.Error("Failed to parse backend service address", log.String("address", address), log.Err(err))
		return nil, err
	}

	tr, err := transport.NewOneWayTLSTransport(cafile, true)
	if err != nil {
		log.Error("Failed to create one-way HTTPS transport", log.String("caFile", cafile), log.Err(err))
		return nil, err
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: u.Scheme, Host: u.Host})
	reverseProxy.Transport = tr
	reverseProxy.ModifyResponse = rewriteBody
	reverseProxy.ErrorLog = log.StdErrLogger()
	re, err := regexp.Compile(manifestPattern)
	if err != nil {
		log.Error("Failed to init harbor manifest pattern")
		return nil, err
	}
	registryLocalClient, err := registryinternalclient.NewForConfig(loopbackConfig)
	if err != nil {
		return nil, err
	}
	registryClient = registryLocalClient

	headers := make(map[string]string)
	headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(
		registryConfig.Security.AdminUsername+":"+registryConfig.Security.AdminPassword),
	)
	cfg := &harbor.Configuration{
		BasePath:      fmt.Sprintf("https://%s/api/v2.0", registryConfig.DomainSuffix),
		DefaultHeader: headers,
		UserAgent:     "Swagger-Codegen/1.0.0/go",
		HTTPClient: &http.Client{
			Transport: tr,
		},
	}
	harborClient = harbor.NewAPIClient(cfg)

	return &handler{reverseProxy, u.Host, externalHost, re}, nil
}

func rewriteBody(resp *http.Response) (err error) {

	ctx := resp.Request.Context()
	host := ctx.Value(HarborContextKey("host"))
	externalHost := ctx.Value(HarborContextKey("exHost"))
	authHeader := resp.Header.Get("www-authenticate")
	if authHeader != "" {
		header := fmt.Sprintf("Bearer realm=\"https://%s/service/token\",service=\"harbor-registry\"", externalHost)
		log.Debug("Modify backend harbor header www-authenticate", log.String("header", header))
		resp.Header.Set("www-authenticate", header)
	}

	locationHeader := resp.Header.Get("location")
	if locationHeader != "" {
		log.Debug("Replace harbor location header", log.String("original host", host.(string)), log.String("tke host", externalHost.(string)))
		resp.Header.Set("location", strings.ReplaceAll(locationHeader, host.(string), externalHost.(string)))
	}

	manifestPattern := ctx.Value(HarborContextKey("manifestPattern"))
	if manifestPattern.(string) == "true" && resp.StatusCode < 300 && resp.StatusCode >= 200 {
		pathSpiltted := strings.Split(resp.Request.URL.Path, "/")
		harborProject := pathSpiltted[2]
		repoName := pathSpiltted[3]
		tagName := pathSpiltted[5]
		tenantID := ctx.Value(HarborContextKey("tenantID"))
		namespaceName := strings.ReplaceAll(harborProject, tenantID.(string)+"-image-", "")
		namespaceList, err := registryClient.Namespaces().List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.tenantID=%s,spec.name=%s", tenantID, namespaceName),
		})
		if err != nil {
			return err
		}
		if len(namespaceList.Items) == 0 {
			return fmt.Errorf("namespace %s in tenant %s not exist", namespaceName, tenantID)
		}
		namespaceObject := namespaceList.Items[0]
		repoList, err := registryClient.Repositories(namespaceObject.ObjectMeta.Name).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.tenantID=%s,spec.name=%s,spec.namespaceName=%s", tenantID, repoName, namespaceName),
		})
		if err != nil {
			return err
		}

		var repoObject *registry.Repository
		if len(repoList.Items) > 0 {

			repoObject = &repoList.Items[0]

		}

		if resp.Request.Method == "PUT" {

			artifact, _, err := harborClient.ArtifactApi.GetArtifact(ctx, harborProject, repoName, tagName, nil)
			if err != nil {
				return fmt.Errorf("harbor artifact /%s/%s/%s not exist", harborProject, repoName, tagName)
			}
			pushRepository(ctx, registryClient, &namespaceObject, repoObject, repoName, tagName, artifact.Digest)

		} else if resp.Request.Method == "GET" {

			pullRepository(ctx, registryClient, &namespaceObject, repoObject, repoName, tagName)

		}

	}

	return nil
}

func pushRepository(ctx context.Context, registryClient *registryinternalclient.RegistryClient, namespace *registry.Namespace, repository *registry.Repository, repoName, tag, digest string) error {
	needIncreaseRepoCount := false
	if repository == nil {
		needIncreaseRepoCount = true
		if _, err := registryClient.Repositories(namespace.ObjectMeta.Name).Create(ctx, &registry.Repository{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.ObjectMeta.Name,
			},
			Spec: registry.RepositorySpec{
				Name:          repoName,
				TenantID:      namespace.Spec.TenantID,
				NamespaceName: namespace.Spec.Name,
				Visibility:    namespace.Spec.Visibility,
			},
			Status: registry.RepositoryStatus{
				PullCount: 0,
				Tags: []registry.RepositoryTag{
					{
						Name:        tag,
						Digest:      digest,
						TimeCreated: metav1.Now(),
					},
				},
			},
		}, metav1.CreateOptions{}); err != nil {
			log.Error("Failed to create repository while received notification",
				log.String("tenantID", namespace.Spec.TenantID),
				log.String("namespace", namespace.Spec.Name),
				log.String("repo", repoName),
				log.String("tag", tag),
				log.Err(err))
			return err
		}
	} else {
		existTag := false
		if len(repository.Status.Tags) == 0 {
			needIncreaseRepoCount = true
		} else {
			for k, v := range repository.Status.Tags {
				if v.Name == tag {
					existTag = true
					repository.Status.Tags[k] = registry.RepositoryTag{
						Name:        tag,
						Digest:      digest,
						TimeCreated: metav1.Now(),
					}
					if _, err := registryClient.Repositories(namespace.ObjectMeta.Name).UpdateStatus(ctx, repository, metav1.UpdateOptions{}); err != nil {
						log.Error("Failed to update repository tag while received notification",
							log.String("tenantID", namespace.Spec.TenantID),
							log.String("namespace", namespace.Spec.Name),
							log.String("repo", repoName),
							log.String("tag", tag),
							log.Err(err))
						return err
					}
					break
				}
			}
		}

		if !existTag {
			repository.Status.Tags = append(repository.Status.Tags, registry.RepositoryTag{
				Name:        tag,
				Digest:      digest,
				TimeCreated: metav1.Now(),
			})
			if _, err := registryClient.Repositories(namespace.ObjectMeta.Name).UpdateStatus(ctx, repository, metav1.UpdateOptions{}); err != nil {
				log.Error("Failed to create repository tag while received notification",
					log.String("tenantID", namespace.Spec.TenantID),
					log.String("namespace", namespace.Spec.Name),
					log.String("repo", repoName),
					log.String("tag", tag),
					log.Err(err))
				return err
			}
		}
	}

	if needIncreaseRepoCount {
		// update namespace repo count
		namespace.Status.RepoCount = namespace.Status.RepoCount + 1
		if _, err := registryClient.Namespaces().UpdateStatus(ctx, namespace, metav1.UpdateOptions{}); err != nil {
			log.Error("Failed to update namespace repo count while received notification",
				log.String("tenantID", namespace.Spec.TenantID),
				log.String("namespace", namespace.Spec.Name),
				log.String("repo", repoName),
				log.String("tag", tag),
				log.Err(err))
			return err
		}
	}
	return nil
}

func pullRepository(ctx context.Context, registryClient *registryinternalclient.RegistryClient, namespace *registry.Namespace, repository *registry.Repository, repoName, tag string) error {
	if repository == nil {
		return fmt.Errorf("repository %s not exist", repoName)
	}
	repository.Status.PullCount = repository.Status.PullCount + 1
	if _, err := registryClient.Repositories(namespace.ObjectMeta.Name).UpdateStatus(ctx, repository, metav1.UpdateOptions{}); err != nil {
		log.Error("Failed to update repository pull count while received notification",
			log.String("tenantID", namespace.Spec.TenantID),
			log.String("namespace", namespace.Spec.Name),
			log.String("repo", repoName),
			log.String("tag", tag),
			log.Err(err))
		return err
	}
	return nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {

	log.Debug("Reverse proxy to backend harbor", log.String("url", req.URL.Path))
	req.Host = h.host
	ctx := context.WithValue(req.Context(), HarborContextKey("host"), h.host)
	ctx = context.WithValue(ctx, HarborContextKey("exHost"), h.externalHost)
	ctx = context.WithValue(ctx, HarborContextKey("manifestPattern"), strconv.FormatBool(h.manifestRegexp.MatchString(req.URL.Path)))

	req = req.WithContext(ctx)
	h.reverseProxy.ServeHTTP(w, req)

}
