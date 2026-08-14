package domain

import "strings"

type GTIDSet string

func (g GTIDSet) Empty() bool { return strings.TrimSpace(string(g)) == "" }

func (g GTIDSet) Contains(other GTIDSet) bool {
	if other.Empty() { return true }
	return strings.Contains(string(g), string(other))
}

type BinlogPosition struct {
	File     string  `json:"file"`
	Position uint64  `json:"position"`
	GTIDSet  GTIDSet `json:"gtid_set,omitempty"`
}

type BinlogMetadata struct {
	EngineVariant     DatabaseEngine `json:"engine_variant"`
	ServerID          string         `json:"server_id"`
	File              string         `json:"file"`
	StartPosition     uint64         `json:"start_position"`
	EndPosition       uint64         `json:"end_position"`
	GTIDSet           string         `json:"gtid_set,omitempty"`
	PreviousGTIDSet   string         `json:"previous_gtid_set,omitempty"`
	ChecksumAlgorithm string         `json:"checksum_algorithm,omitempty"`
}
