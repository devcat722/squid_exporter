package types

/*VarLabel maps key value prometheus labes*/
type VarLabel struct {
	Key   string
	Value string
}

/*Counter maps a squid conters */
type Counter struct {
	Key       string
	Value     float64
	VarLabels []VarLabel
}

/*Counters is a list of multiple squid counters */
type Counters []Counter

type MemInstance struct {
	Key   string
	KID   string
	Pool  string
	Value float64
}

type MemInstances []MemInstance
