package metrics

import "sync"

// This file implements a registry for metrics.
// The purpose of the registry is to make it possible to instantiate multiple metrics based on their names only.
// We distinguish between client metrics and replica metrics, but a single metric could work on both.

var (
	registryMut    sync.Mutex
	sourcerMetrics = map[string]func() any{}
	clientMetrics  = map[string]func() any{}
	replicaMetrics = map[string]func() any{}
)

// RegisterSourcerMetric registers the constructor of a sourcer metric.
func RegisterSourcerMetric(name string, constructor func() any) {
	registryMut.Lock()
	defer registryMut.Unlock()
	sourcerMetrics[name] = constructor
}

// RegisterClientMetric registers the constructor of a client metric.
func RegisterClientMetric(name string, constructor func() any) {
	registryMut.Lock()
	defer registryMut.Unlock()
	clientMetrics[name] = constructor
}

// RegisterReplicaMetric registers the constructor of a replica metric.
func RegisterReplicaMetric(name string, constructor func() any) {
	registryMut.Lock()
	defer registryMut.Unlock()
	replicaMetrics[name] = constructor
}

// GetSourcerMetrics constructs a new instance of each named metric.
func GetSourcerMetrics(names ...string) (metrics []any) {
	registryMut.Lock()
	defer registryMut.Unlock()

	for _, name := range names {
		if constructor, ok := sourcerMetrics[name]; ok {
			metrics = append(metrics, constructor())
		}
	}
	return
}

// GetClientMetrics constructs a new instance of each named metric.
func GetClientMetrics(names ...string) (metrics []any) {
	registryMut.Lock()
	defer registryMut.Unlock()

	for _, name := range names {
		if constructor, ok := clientMetrics[name]; ok {
			metrics = append(metrics, constructor())
		}
	}
	return
}

// GetReplicaMetrics constructs a new instance of each named metric.
func GetReplicaMetrics(names ...string) (metrics []any) {
	registryMut.Lock()
	defer registryMut.Unlock()

	for _, name := range names {
		if constructor, ok := replicaMetrics[name]; ok {
			metrics = append(metrics, constructor())
		}
	}
	return
}
