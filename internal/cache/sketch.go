package cache

import (
	"hash/fnv"
	"math/rand"
	"time"
)

type CountMinSketch struct {
	width int
	depth int
	table [][]uint32
	count uint64
	r     *rand.Rand
}

func NewSketch(width, depth int) *CountMinSketch {
	table := make([][]uint32, depth)
	for i := range table {
		table[i] = make([]uint32, width)
	}
	return &CountMinSketch{
		width: width,
		depth: depth,
		table: table,
		r:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *CountMinSketch) Add(key string) {
	s.count++
	if s.count > uint64(s.width*s.depth*10) {
		s.Reset()
	}

	h := fnvHash(key)
	for i := 0; i < s.depth; i++ {
		idx := (h + uint64(i)*12345) % uint64(s.width)
		s.table[i][idx]++
	}
}

func (s *CountMinSketch) Estimate(key string) uint32 {
	min := ^uint32(0)
	h := fnvHash(key)
	for i := 0; i < s.depth; i++ {
		idx := (h + uint64(i)*12345) % uint64(s.width)
		if s.table[i][idx] < min {
			min = s.table[i][idx]
		}
	}
	return min
}

func (s *CountMinSketch) Reset() {
	for i := 0; i < s.depth; i++ {
		for j := 0; j < s.width; j++ {
			s.table[i][j] /= 2
		}
	}

	s.count /= 2
}

func fnvHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))

	return h.Sum64()
}
