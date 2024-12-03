package collector

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
)

type squidMem struct {
	Section     string
	Description string
}

var squidMems = []squidMem{
	{"obj_size_bytes", "Size of each object in the pool in bytes"},
	{"chunks_kb_per_chunk", "Chunk size in kilobytes"},
	{"objs_per_chunk", "Number of objects per chunk"},
	{"alloc_bytes", "Memory allocated for each mempool"},
	{"inuse_bytes", "Memory currently in use for each mempool"},
	{"idle_bytes", "Memory currently idle in each mempool"},
	{"fragmentation_pct", "Fragmentation percentage in each mempool"},
	{"allocation_rate_per_sec", "Memory allocation rate for each mempool"},
}

func sanitizeMetricName(name string) string {
	// Replace invalid characters with underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9_:]`)
	return re.ReplaceAllString(name, "_")
}

func generateSquidMems(labels []string) descMap {
	Mems := descMap{}

	for i := range squidMems {
		Mem := squidMems[i]
		var key = Mem.Section
		if _, exists := Mems[key]; !exists {
			Mems[key] = prometheus.NewDesc(
				prometheus.BuildFQName(namespace, "mempool", Mem.Section),
				Mem.Description,
				[]string{"k_id", "pool"}, // Define labels here
				nil,
			)
		}
	}

	return Mems
}
