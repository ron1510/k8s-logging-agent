// Package partitioner determines shard ownership for pods.
package partitioner

import "hash/fnv"

// Partitioner assigns each pod UID to exactly one shard.
type Partitioner struct {
	total   int
	ordinal int
}

// New constructs a Partitioner. Inputs are assumed validated by config.
func New(total, ordinal int) Partitioner {
	return Partitioner{total: total, ordinal: ordinal}
}

// OwnsPodUID returns true when this shard owns the given pod UID.
func (p Partitioner) OwnsPodUID(podUID string) bool {
	if p.total <= 1 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(podUID))
	return int(h.Sum32()%uint32(p.total)) == p.ordinal
}
