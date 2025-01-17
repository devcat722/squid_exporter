package collector

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/boynux/squid-exporter/types"
)

var kidType string = "kid"
var kidId int = 0

/*CacheObjectClient holds information about squid manager */
type CacheObjectClient struct {
	ch              connectionHandler
	basicAuthString string
	headers         []string
}

type CacheMemoryClient struct {
	ch              connectionHandler
	basicAuthString string
	headers         []string
}
type connectionHandler interface {
	connect() (net.Conn, error)
}

type connectionHandlerImpl struct {
	hostname string
	port     int
}

/*SquidClient provides functionality to fetch squid metrics */
type SquidClient interface {
	GetCounters() (types.Counters, error)
	GetServiceTimes() (types.Counters, error)
	GetInfos() (types.Counters, error)
}
type MemClient interface {
	GetMems() (types.MemInstances, error)
}

const (
	requestProtocol = "GET cache_object://localhost/%s HTTP/1.0"
)

func buildBasicAuthString(login string, password string) string {
	if len(login) == 0 {
		return ""
	} else {
		return base64.StdEncoding.EncodeToString([]byte(login + ":" + password))
	}
}

type CacheObjectRequest struct {
	Hostname string
	Port     int
	Login    string
	Password string
	Headers  []string
}

/*NewCacheObjectClient initializes a new cache client */
func NewCacheObjectClient(cor *CacheObjectRequest) *CacheObjectClient {
	return &CacheObjectClient{
		&connectionHandlerImpl{
			cor.Hostname,
			cor.Port,
		},
		buildBasicAuthString(cor.Login, cor.Password),
		cor.Headers,
	}
}

// NewCacheMemoryClient creates a new instance of CacheMemoryClient
func NewCacheMemoryClient(cor *CacheObjectRequest) *CacheMemoryClient {
	// return &CacheMemoryClient{request: req}

	return &CacheMemoryClient{
		&connectionHandlerImpl{
			cor.Hostname,
			cor.Port,
		},
		buildBasicAuthString(cor.Login, cor.Password),
		cor.Headers,
	}
}

func (c *CacheObjectClient) readFromSquid(endpoint string) (*bufio.Reader, error) {
	conn, err := c.ch.connect()

	if err != nil {
		return nil, err
	}
	r, err := get(conn, endpoint, c.basicAuthString, c.headers)

	if err != nil {
		return nil, err
	}

	if r.StatusCode != 200 {
		return nil, fmt.Errorf("Non success code %d while fetching metrics", r.StatusCode)
	}

	return bufio.NewReader(r.Body), err
}

func (c *CacheMemoryClient) readFromSquidMem(endpoint string) (*bufio.Reader, error) {
	conn, err := c.ch.connect()

	if err != nil {
		return nil, err
	}
	r, err := get(conn, endpoint, c.basicAuthString, c.headers)

	if err != nil {
		return nil, err
	}

	if r.StatusCode != 200 {
		return nil, fmt.Errorf("Non success code %d while fetching metrics", r.StatusCode)
	}

	return bufio.NewReader(r.Body), err
}

func readLines(reader *bufio.Reader, lines chan<- string) {
	for {
		line, err := reader.ReadString('\n')

		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("error reading from the bufio.Reader: %v", err)
			break
		}
		// log.Printf("Line read: %s", line)
		lines <- line
	}
	close(lines)
}

/*GetCounters fetches counters from squid cache manager */
func (c *CacheObjectClient) GetCounters() (types.Counters, error) {
	var counters types.Counters

	reader, err := c.readFromSquid("counters")
	if err != nil {
		return nil, fmt.Errorf("error getting counters: %v", err)
	}

	lines := make(chan string)
	go readLines(reader, lines)

	for line := range lines {
		// log.Printf("Processing line: %s", line)
		c, err := decodeCounterStrings(line)
		if err != nil {
			log.Println(err)
		} else {
			counters = append(counters, c)
		}
	}

	return counters, err
}

/*GetMems fetches Memory pool from squid cache manager */
func (c *CacheMemoryClient) GetMems() (types.MemInstances, error) {
	var Mems types.MemInstances
	reader, err := c.readFromSquidMem("mem")

	if err != nil {
		return nil, fmt.Errorf("error getting Mempools: %v", err)
	}

	lines := make(chan string)
	go readLines(reader, lines)
	kidId = 0
	// fileName := "mem1.txt"

	// // Open the file
	// file, err := os.Open(fileName)
	// if err != nil {
	// 	log.Fatalf("Failed to open file: %v", err)
	// }
	// defer file.Close()

	// // Create a slice to hold all lines
	// var lines []string

	// // Use a scanner to read the file line by line
	// scanner := bufio.NewScanner(file)
	// for scanner.Scan() {
	// 	line := scanner.Text() // Read the current line
	// 	lines = append(lines, line)
	// }

	var aggregatedMetrics = make(map[string]types.MemInstance)

	for line := range lines {
		// log.Printf("Processing line: %s", line)
		c, err := decodeMemStrings(line)
		if err != nil {
			log.Println(err)
		} else {
			for i := 0; i < len(c.VarLabels); i++ {
				var memTemp types.MemInstance

				memTemp.KID = kidType
				memTemp.Pool = c.Key
				memValue, err := strconv.ParseFloat(c.VarLabels[i].Value, 64)
				memTemp.Value = memValue
				memTemp.Key = c.VarLabels[i].Key
				if err == nil {
				}

				uniqueKey := fmt.Sprintf("%s_%s_%s", memTemp.Key, memTemp.KID, memTemp.Pool)

				// Aggregate the values if the metric already exists
				if existing, exists := aggregatedMetrics[uniqueKey]; exists {
					existing.Value += memValue
					aggregatedMetrics[uniqueKey] = existing
				} else {
					memTemp.Value = memValue
					aggregatedMetrics[uniqueKey] = memTemp
				}
			}
		}
	}

	Mems = make([]types.MemInstance, 0, len(aggregatedMetrics))
	for _, mem := range aggregatedMetrics {
		Mems = append(Mems, mem)
	}

	log.Printf("kidType: %s", kidType)
	// log.Printf("Processed Memory Pools: %+v", Mems)
	return Mems, err
}

/*GetServiceTimes fetches service times from squid cache manager */
func (c *CacheObjectClient) GetServiceTimes() (types.Counters, error) {
	var serviceTimes types.Counters

	reader, err := c.readFromSquid("service_times")
	if err != nil {
		return nil, fmt.Errorf("error getting service times: %v", err)
	}

	lines := make(chan string)
	go readLines(reader, lines)

	for line := range lines {
		s, err := decodeServiceTimeStrings(line)
		if err != nil {
			log.Println(err)
		} else {
			if s.Key != "" {
				serviceTimes = append(serviceTimes, s)
			}
		}
	}

	return serviceTimes, err
}

/*GetInfos fetches info from squid cache manager */
func (c *CacheObjectClient) GetInfos() (types.Counters, error) {
	var infos types.Counters

	reader, err := c.readFromSquid("info")
	if err != nil {
		return nil, fmt.Errorf("error getting info: %v", err)
	}

	lines := make(chan string)
	go readLines(reader, lines)

	var infoVarLabels types.Counter
	infoVarLabels.Key = "squid_info"
	infoVarLabels.Value = 1

	for line := range lines {
		dis, err := decodeInfoStrings(line)
		if err != nil {
			log.Println(err)
		} else {
			if len(dis.VarLabels) > 0 {
				if dis.VarLabels[0].Key == "5min" {
					var infoAvg5 types.Counter
					var infoAvg60 types.Counter

					infoAvg5.Key = dis.Key + "_" + dis.VarLabels[0].Key
					infoAvg60.Key = dis.Key + "_" + dis.VarLabels[1].Key

					if value, err := strconv.ParseFloat(dis.VarLabels[0].Value, 64); err == nil {
						infoAvg5.Value = value
						infos = append(infos, infoAvg5)
					}
					if value, err := strconv.ParseFloat(dis.VarLabels[1].Value, 64); err == nil {
						infoAvg60.Value = value
						infos = append(infos, infoAvg60)
					}

				} else {
					infoVarLabels.VarLabels = append(infoVarLabels.VarLabels, dis.VarLabels[0])
				}
			} else if dis.Key != "" {
				infos = append(infos, dis)
			}
		}
	}
	infos = append(infos, infoVarLabels)
	// log.Printf("hey: %s", infos)
	return infos, err
}

func (ch *connectionHandlerImpl) connect() (net.Conn, error) {
	return net.Dial("tcp", fmt.Sprintf("%s:%d", ch.hostname, ch.port))
}

func get(conn net.Conn, path string, basicAuthString string, headers []string) (*http.Response, error) {
	rBody := append(headers, []string{
		fmt.Sprintf(requestProtocol, path),
		"Host: localhost",
		"User-Agent: squidclient/3.5.12",
	}...)

	if len(basicAuthString) > 0 {
		rBody = append(rBody, "Proxy-Authorization: Basic "+basicAuthString)
		rBody = append(rBody, "Authorization: Basic "+basicAuthString)
	}
	rBody = append(rBody, "Accept: */*", "\r\n")
	request := strings.Join(rBody, "\r\n")

	fmt.Fprint(conn, request)

	return http.ReadResponse(bufio.NewReader(conn), nil)
}

func decodeCounterStrings(line string) (types.Counter, error) {
	if equal := strings.Index(line, "="); equal >= 0 {
		if key := strings.TrimSpace(line[:equal]); len(key) > 0 {
			value := ""
			if len(line) > equal {
				value = strings.TrimSpace(line[equal+1:])
			}

			// Remove additional formating string from `sample_time`
			if slices := strings.Split(value, " "); len(slices) > 0 {
				value = slices[0]
			}

			if i, err := strconv.ParseFloat(value, 64); err == nil {
				return types.Counter{Key: key, Value: i}, nil
			}
		}
	}

	return types.Counter{}, errors.New("counter - could not parse line: " + line)
}

func decodeMemStrings(line string) (types.Counter, error) {
	// Skip non-metric lines (e.g., headers, summary text)
	if strings.Contains(line, "Obj Size") {
		kidId = kidId + 1
		kidType = "kid" + strconv.Itoa(kidId)
	}

	if strings.HasSuffix(line, ":\n") || strings.HasPrefix(line, "by kid") || strings.HasPrefix(line, "Total Pools") || strings.HasPrefix(line, "Cumulative") {
		return types.Counter{}, nil
	}

	// For table-style data rows
	fields := strings.Fields(line)
	if len(fields) >= 18 {
		// The first field is typically the key
		log.Printf("%d", len(fields))
		j := 0
		for i := 0; i < len(fields); i++ {
			log.Printf(" %s", fields[i])
			if j == 0 && len(fields[i]) > 0 && unicode.IsDigit(rune(fields[i][0])) {
				j = i
			}
		}
		log.Printf("%d", j)

		var key string
		if j == 1 {
			key = fields[0]
		} else if j == 2 {
			key = fields[0] + "_" + fields[1]
		} else {
			key = fields[0] + "_" + fields[1]
		}

		var memCounter types.Counter
		var memVarLabel types.VarLabel

		memVarLabel.Key = "obj_size_bytes"
		memVarLabel.Value = fields[1+j-1]
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		memVarLabel.Key = "chunks_kb_per_chunk"
		memVarLabel.Value = fields[3+j-1]
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		memVarLabel.Key = "objs_per_chunk"
		memVarLabel.Value = fields[2+j-1]
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		memVarLabel.Key = "alloc_bytes"
		memVarLabel.Value = fields[3+j-1]
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		memVarLabel.Key = "inuse_bytes"
		memVarLabel.Value = fields[8+j-1]
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		memVarLabel.Key = "idle_bytes"
		memVarLabel.Value = fields[13+j-1]
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		memVarLabel.Key = "fragmentation_pct"
		memoryInUse, err := strconv.ParseFloat(fields[8+j-1], 64)
		if err != nil {
			memoryInUse = 0
		}
		memoryAllocated, err := strconv.ParseFloat(fields[3+j-1], 64)
		if err != nil {
			memoryAllocated = 1
		}
		fragmentationPercentage := (1 - (1.0 * memoryInUse / memoryAllocated)) * 100
		strValue := fmt.Sprintf("%.1f", fragmentationPercentage)
		memVarLabel.Value = strValue
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		memVarLabel.Key = "allocation_rate_per_sec"
		memVarLabel.Value = fields[18+j-1]
		memCounter.VarLabels = append(memCounter.VarLabels, memVarLabel)

		// The last field or a specific column contains the value
		value := fields[len(fields)-1]
		if numericValue, err := strconv.ParseFloat(value, 64); err == nil {
			return types.Counter{Key: key, Value: numericValue, VarLabels: memCounter.VarLabels}, nil
		}
	}

	// Return an error if parsing fails
	return types.Counter{}, errors.New("decodeMemStrings - could not parse line: " + line)
}

func decodeServiceTimeStrings(line string) (types.Counter, error) {
	if strings.HasSuffix(line, ":\n") { // A header line isn't a metric
		return types.Counter{}, nil
	}
	if equal := strings.Index(line, ":"); equal >= 0 {
		if key := strings.TrimSpace(line[:equal]); len(key) > 0 {
			value := ""
			if len(line) > equal {
				value = strings.TrimSpace(line[equal+1:])
			}
			key = strings.Replace(key, " ", "_", -1)
			key = strings.Replace(key, "(", "", -1)
			key = strings.Replace(key, ")", "", -1)

			if equalTwo := strings.Index(value, "%"); equalTwo >= 0 {
				if keyTwo := strings.TrimSpace(value[:equalTwo]); len(keyTwo) > 0 {
					if len(value) > equalTwo {
						value = strings.Split(strings.TrimSpace(value[equalTwo+1:]), " ")[0]
					}
					key = key + "_" + keyTwo
				}
			}

			if value, err := strconv.ParseFloat(value, 64); err == nil {
				return types.Counter{Key: key, Value: value}, nil
			}
		}
	}

	return types.Counter{}, errors.New("service times - could not parse line: " + line)
}

func decodeInfoStrings(line string) (types.Counter, error) {
	if strings.HasSuffix(line, ":\n") { // A header line isn't a metric
		return types.Counter{}, nil
	}

	if idx := strings.Index(line, ":"); idx >= 0 { // detect if line contain metric format like "metricName: value"
		if key := strings.TrimSpace(line[:idx]); len(key) > 0 {
			value := ""
			if len(line) > idx {
				value = strings.TrimSpace(line[idx+1:])
			}
			key = strings.Replace(key, " ", "_", -1)
			key = strings.Replace(key, "(", "", -1)
			key = strings.Replace(key, ")", "", -1)
			key = strings.Replace(key, ",", "", -1)
			key = strings.Replace(key, "/", "", -1)

			// metrics with value as string need to save as label, format like "Squid Object Cache: Version 6.1" (the 3 first metrics)
			if key == "Squid_Object_Cache" || key == "Build_Info" || key == "Service_Name" {
				if key == "Squid_Object_Cache" { // To clarify that the value is the squid version.
					key = key + "_Version"
					if slices := strings.Split(value, " "); len(slices) > 0 {
						value = slices[1]
					}
				}
				var infoVarLabel types.VarLabel
				infoVarLabel.Key = key
				infoVarLabel.Value = value

				var infoCounter types.Counter
				infoCounter.Key = key
				infoCounter.VarLabels = append(infoCounter.VarLabels, infoVarLabel)
				return infoCounter, nil
			} else if key == "Start_Time" || key == "Current_Time" { // discart this metrics
				return types.Counter{}, nil
			}

			// Remove additional information in value metric
			if slices := strings.Split(value, " "); len(slices) > 0 {
				if slices[0] == "5min:" && slices[2] == "60min:" { // catch metrics with avg in 5min and 60min format like "Hits as % of bytes sent: 5min: -0.0%, 60min: -0.0%"
					var infoAvg5mVarLabel types.VarLabel
					infoAvg5mVarLabel.Key = slices[0]
					infoAvg5mVarLabel.Value = slices[1]

					infoAvg5mVarLabel.Key = strings.Replace(infoAvg5mVarLabel.Key, ":", "", -1)
					infoAvg5mVarLabel.Value = strings.Replace(infoAvg5mVarLabel.Value, "%", "", -1)
					infoAvg5mVarLabel.Value = strings.Replace(infoAvg5mVarLabel.Value, ",", "", -1)

					var infoAvg60mVarLabel types.VarLabel
					infoAvg60mVarLabel.Key = slices[2]
					infoAvg60mVarLabel.Value = slices[3]

					infoAvg60mVarLabel.Key = strings.Replace(infoAvg60mVarLabel.Key, ":", "", -1)
					infoAvg60mVarLabel.Value = strings.Replace(infoAvg60mVarLabel.Value, "%", "", -1)
					infoAvg60mVarLabel.Value = strings.Replace(infoAvg60mVarLabel.Value, ",", "", -1)

					var infoAvgCounter types.Counter
					infoAvgCounter.Key = key
					infoAvgCounter.VarLabels = append(infoAvgCounter.VarLabels, infoAvg5mVarLabel, infoAvg60mVarLabel)

					return infoAvgCounter, nil
				} else {
					value = slices[0]
				}

			}

			value = strings.Replace(value, "%", "", -1)
			value = strings.Replace(value, ",", "", -1)
			// log.Printf("my output: %s", value)

			if i, err := strconv.ParseFloat(value, 64); err == nil {
				return types.Counter{Key: key, Value: i}, nil
			}
		}
	} else {
		// this catch the last 4 metrics format like "value metricName"
		lineTrimed := strings.TrimSpace(line[:])

		if idx := strings.Index(lineTrimed, " "); idx >= 0 {
			key := strings.TrimSpace(lineTrimed[idx+1:])
			key = strings.Replace(key, " ", "_", -1)
			key = strings.Replace(key, "-", "_", -1)

			value := strings.TrimSpace(lineTrimed[:idx])

			if i, err := strconv.ParseFloat(value, 64); err == nil {
				return types.Counter{Key: key, Value: i}, nil
			}
		}
	}
	return types.Counter{}, errors.New("Info - could not parse line: " + line)
}
