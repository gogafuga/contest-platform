package task_03

import (
	"encoding/binary"
	"math/rand"
	"net/netip"
)

// Generate constructs only the fixed-seed dataset for this task.
func Generate() (dataset []string) {
	rng := rand.New(rand.NewSource(20261111))
	prefix := netip.MustParsePrefix(allowedCIDR)
	bytes := prefix.Addr().As4()
	first := binary.BigEndian.Uint32(bytes[:])
	size := uint32(1) << uint(32-prefix.Bits())
	last := first + size - 1
	queries := []uint32{first - 1, first, first + 1, last - 1, last, last + 1, first - 1, last + 1, 0, ^uint32(0)}
	for i := 0; i < 5000; i++ {
		var value uint32
		switch i % 3 {
		case 0:
			value = first + uint32(rng.Int63n(int64(size)))
		case 1:
			value = first - size + uint32(rng.Int63n(int64(3*size)))
		default:
			value = rng.Uint32()
		}
		queries = append(queries, value)
	}
	rng.Shuffle(len(queries), func(i, j int) { queries[i], queries[j] = queries[j], queries[i] })
	for _, value := range queries {
		var bytes [4]byte
		binary.BigEndian.PutUint32(bytes[:], value)
		dataset = append(dataset, netip.AddrFrom4(bytes).String())
	}
	return dataset
}
