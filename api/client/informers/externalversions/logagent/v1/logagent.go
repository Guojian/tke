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

// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	time "time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
	versioned "tkestack.io/tke/api/client/clientset/versioned"
	internalinterfaces "tkestack.io/tke/api/client/informers/externalversions/internalinterfaces"
	v1 "tkestack.io/tke/api/client/listers/logagent/v1"
	logagentv1 "tkestack.io/tke/api/logagent/v1"
)

// LogAgentInformer provides access to a shared informer and lister for
// LogAgents.
type LogAgentInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.LogAgentLister
}

type logAgentInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewLogAgentInformer constructs a new informer for LogAgent type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewLogAgentInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredLogAgentInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredLogAgentInformer constructs a new informer for LogAgent type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredLogAgentInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.LogagentV1().LogAgents().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.LogagentV1().LogAgents().Watch(options)
			},
		},
		&logagentv1.LogAgent{},
		resyncPeriod,
		indexers,
	)
}

func (f *logAgentInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredLogAgentInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *logAgentInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&logagentv1.LogAgent{}, f.defaultInformer)
}

func (f *logAgentInformer) Lister() v1.LogAgentLister {
	return v1.NewLogAgentLister(f.Informer().GetIndexer())
}