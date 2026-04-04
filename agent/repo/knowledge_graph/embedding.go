package knowledgegraph

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeEmbeddingF32 packs a float32 slice as little-endian bytes for SQLite BLOB.
func EncodeEmbeddingF32(v []float32) ([]byte, error) {
	if len(v) == 0 {
		return nil, fmt.Errorf("knowledgegraph: EncodeEmbeddingF32: empty vector")
	}
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b, nil
}

// DecodeEmbeddingF32 unpacks a BLOB from EncodeEmbeddingF32.
func DecodeEmbeddingF32(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("knowledgegraph: DecodeEmbeddingF32: length %d not multiple of 4", len(b))
	}
	n := len(b) / 4
	out := make([]float32, n)
	for i := range n {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out, nil
}

// CosineSimilarity returns the cosine of the angle between a and b, or -1 if lengths differ or vectors are zero.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return -1
	}
	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}
	if na == 0 || nb == 0 {
		return -1
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
