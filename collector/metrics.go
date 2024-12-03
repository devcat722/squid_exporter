package collector

import (
	"log"
	"time"

	"github.com/boynux/squid-exporter/config"
	"github.com/prometheus/client_golang/prometheus"
)

type descMap map[string]*prometheus.Desc

const (
	namespace = "squid"
	timeout   = 10 * time.Second
)

var (
	counters            descMap
	serviceTimes        descMap // ExtractServiceTimes decides if we want to extract service times
	ExtractServiceTimes bool
	ExtractMemPools     bool
	infos               descMap
	mems                descMap
)

/*Exporter entry point to squid exporter */
type Exporter struct {
	client    SquidClient
	memClient MemClient

	hostname string
	port     int

	labels config.Labels
	up     *prometheus.GaugeVec
}

type CollectorConfig struct {
	Hostname string
	Port     int
	Login    string
	Password string
	Labels   config.Labels
	Headers  []string
}

/*New initializes a new exporter */
func New(c *CollectorConfig) *Exporter {
	counters = generateSquidCounters(c.Labels.Keys)
	if ExtractServiceTimes {
		serviceTimes = generateSquidServiceTimes(c.Labels.Keys)
	}

	infos = generateSquidInfos(c.Labels.Keys)

	if ExtractMemPools {
		mems = generateSquidMems(c.Labels.Keys)
	}

	return &Exporter{
		client: NewCacheObjectClient(&CacheObjectRequest{
			c.Hostname,
			c.Port,
			c.Login,
			c.Password,
			c.Headers,
		}),
		memClient: NewCacheMemoryClient(&CacheObjectRequest{
			Hostname: c.Hostname,
			Port:     c.Port,
			Login:    c.Login,
			Password: c.Password,
			Headers:  c.Headers,
		}),

		hostname: c.Hostname,
		port:     c.Port,
		labels:   c.Labels,
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the last query of squid successful?",
		}, []string{"host"}),
	}
}

// Describe describes all the metrics ever exported by the ECS exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.up.Describe(ch)

	for _, v := range counters {
		ch <- v
	}

	if ExtractServiceTimes {
		for _, v := range serviceTimes {
			ch <- v
		}
	}

	for _, v := range infos {
		ch <- v
	}

	if ExtractMemPools {
		for _, v := range mems {
			ch <- v
		}
	}

}

/*Collect fetches metrics from squid manager and pushes them to promethus */
func (e *Exporter) Collect(c chan<- prometheus.Metric) {
	insts, err := e.client.GetCounters()

	if err == nil {
		e.up.With(prometheus.Labels{"host": e.hostname}).Set(1)
		for i := range insts {
			if d, ok := counters[insts[i].Key]; ok {
				c <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, insts[i].Value, e.labels.Values...)
			}
		}
	} else {
		e.up.With(prometheus.Labels{"host": e.hostname}).Set(0)
		log.Println("Could not fetch counter metrics from squid instance: ", err)
	}

	if ExtractMemPools {
		memInsts, err := e.memClient.GetMems()
		log.Printf("insts: %v", memInsts)

		if err == nil {
			for i := range memInsts {
				if d, ok := mems[memInsts[i].Key]; ok {
					c <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, memInsts[i].Value, memInsts[i].KID, memInsts[i].Pool)
				}
			}
		}
	}

	if ExtractServiceTimes {
		insts, err = e.client.GetServiceTimes()

		if err == nil {
			for i := range insts {
				if d, ok := serviceTimes[insts[i].Key]; ok {
					c <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, insts[i].Value, e.labels.Values...)
				}
			}
		} else {
			log.Println("Could not fetch service times metrics from squid instance: ", err)
		}
	}

	insts, err = e.client.GetInfos()
	if err == nil {
		for i := range insts {
			if d, ok := infos[insts[i].Key]; ok {
				c <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, insts[i].Value, e.labels.Values...)
			} else if insts[i].Key == "squid_info" {
				infoMetricName := prometheus.BuildFQName(namespace, "info", "service")
				var labelsKeys []string
				var labelsValues []string

				for z := range insts[i].VarLabels {
					labelsKeys = append(labelsKeys, insts[i].VarLabels[z].Key)
					labelsValues = append(labelsValues, insts[i].VarLabels[z].Value)
				}

				infoDesc := prometheus.NewDesc(
					infoMetricName,
					"Metrics as string from info on cache_object",
					labelsKeys,
					nil,
				)
				c <- prometheus.MustNewConstMetric(infoDesc, prometheus.GaugeValue, insts[i].Value, labelsValues...)
			}
		}
	} else {
		log.Println("Could not fetch info metrics from squid instance: ", err)
	}

	e.up.Collect(c)
}
