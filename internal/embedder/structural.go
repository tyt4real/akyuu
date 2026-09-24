package embedder

// AverageVectors computes an unweighted mean of one or more vectors and
// re-normalizes the result to unit length (matching pool.go's convention,
// so structural embeddings stay comparable to content embeddings via
// cosine distance).
func AverageVectors(vecs ...[]float32) []float32 {
	if len(vecs) == 0 {
		return nil
	}
	dim := len(vecs[0])
	out := make([]float32, dim)
	for _, v := range vecs {
		for i, x := range v {
			out[i] += x
		}
	}
	for i := range out {
		out[i] /= float32(len(vecs))
	}
	return normalize(out)
}
