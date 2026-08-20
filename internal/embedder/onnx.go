package embedder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	onnxruntime_go "github.com/yalue/onnxruntime_go"
)

// modelFileNames are the artifacts expected under the configured model_dir.
const (
	onnxFileName      = "model.onnx"
	tokenizerFileName = "tokenizer.json"
)

// ONNX runs a sentence-embedding model in-process via the ONNX Runtime shared
// library (dlopened at runtime). It loads a HuggingFace sentence-transformers
// export: BERT-family inputs (input_ids / attention_mask / token_type_ids) and
// a last_hidden_state (or sentence_embedding) output, mean-pooled and L2
// normalized to produce the stored vector.
type ONNX struct {
	session    *onnxruntime_go.DynamicAdvancedSession
	tokenizer  *wordPiece
	inputNames []string
	outputName string
	dim        int
	version    string
	mu         sync.Mutex // onnxruntime Run calls on one session are serialized
}

var (
	envOnce    sync.Once
	envInitErr error
)

// NewONNX loads the model and tokenizer from modelDir and prepares a session.
// modelVersion is stored with every vector for model isolation at search time.
func NewONNX(modelDir, modelVersion string, dim int) (*ONNX, error) {
	modelPath := filepath.Join(modelDir, onnxFileName)
	tokenizerPath := filepath.Join(modelDir, tokenizerFileName)
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("embedder: model.onnx not found in %s: %w", modelDir, err)
	}
	if _, err := os.Stat(tokenizerPath); err != nil {
		return nil, fmt.Errorf("embedder: tokenizer.json not found in %s: %w", modelDir, err)
	}

	tok, err := loadTokenizer(tokenizerPath)
	if err != nil {
		return nil, err
	}

	envOnce.Do(func() {
		// The shared library (libonnxruntime.so) is loaded lazily at runtime.
		// ORT_LIBRARY_PATH points at an explicit location; otherwise the
		// wrapper looks up "onnxruntime.so" via the dynamic loader.
		if path := os.Getenv("ORT_LIBRARY_PATH"); path != "" {
			onnxruntime_go.SetSharedLibraryPath(path)
		}
		envInitErr = onnxruntime_go.InitializeEnvironment()
	})
	if envInitErr != nil {
		return nil, fmt.Errorf("embedder: initialize onnxruntime: %w", envInitErr)
	}

	inputs, outputs, err := onnxruntime_go.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("embedder: inspect model: %w", err)
	}

	o := &ONNX{dim: dim, version: modelVersion, tokenizer: tok}
	for _, in := range inputs {
		switch in.Name {
		case "input_ids", "attention_mask", "token_type_ids":
			o.inputNames = append(o.inputNames, in.Name)
		}
	}
	if len(o.inputNames) == 0 {
		return nil, fmt.Errorf("embedder: model %s has no BERT-style inputs", modelPath)
	}
	o.outputName = pickOutput(outputs)
	if o.outputName == "" {
		return nil, fmt.Errorf("embedder: model %s has no usable tensor output", modelPath)
	}

	options, err := onnxruntime_go.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("embedder: session options: %w", err)
	}
	defer options.Destroy()

	session, err := onnxruntime_go.NewDynamicAdvancedSession(
		modelPath, o.inputNames, []string{o.outputName}, options)
	if err != nil {
		return nil, fmt.Errorf("embedder: load session: %w", err)
	}
	o.session = session
	return o, nil
}

// pickOutput prefers a hidden-state output and falls back to the first tensor
// output, so both last_hidden_state and sentence_embedding exports work.
func pickOutput(outputs []onnxruntime_go.InputOutputInfo) string {
	for _, out := range outputs {
		if out.Name == "last_hidden_state" || out.Name == "sentence_embedding" {
			return out.Name
		}
	}
	for _, out := range outputs {
		if out.OrtValueType == onnxruntime_go.ONNXTypeTensor {
			return out.Name
		}
	}
	return ""
}

// Close releases the session. Call once when the process is done with it.
func (o *ONNX) Close() error {
	if o.session == nil {
		return nil
	}
	return o.session.Destroy()
}

// EmbedBatch implements Embedder.
func (o *ONNX) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	seqs := make([][]int64, 0, len(texts))
	masks := make([][]int64, 0, len(texts))
	idsBatches := make([][]int64, 0, len(texts))
	tts := make([][]int64, 0, len(texts))

	for _, text := range texts {
		ids, mask := o.tokenizer.tokenize(text)
		mask64 := make([]int64, len(mask))
		copy(mask64, mask)
		seqs = append(seqs, ids)
		masks = append(masks, mask64)
		idsBatches = append(idsBatches, ids)
		tts = append(tts, make([]int64, len(ids)))
	}

	maxSeq := 0
	for _, s := range seqs {
		if len(s) > maxSeq {
			maxSeq = len(s)
		}
	}
	batch := len(texts)
	if maxSeq == 0 || batch == 0 {
		return nil, fmt.Errorf("embedder: empty batch")
	}

	flatIDs := flattenInt64(seqs, maxSeq)
	flatMask := flattenInt64(masks, maxSeq)
	flatTT := flattenInt64(tts, maxSeq)

	inputTensors := make([]onnxruntime_go.Value, 0, len(o.inputNames))
	for _, name := range o.inputNames {
		var t *onnxruntime_go.Tensor[int64]
		var err error
		switch name {
		case "input_ids":
			t, err = onnxruntime_go.NewTensor(onnxruntime_go.Shape{int64(batch), int64(maxSeq)}, flatIDs)
		case "attention_mask":
			t, err = onnxruntime_go.NewTensor(onnxruntime_go.Shape{int64(batch), int64(maxSeq)}, flatMask)
		case "token_type_ids":
			t, err = onnxruntime_go.NewTensor(onnxruntime_go.Shape{int64(batch), int64(maxSeq)}, flatTT)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("embedder: input tensor %s: %w", name, err)
		}
		inputTensors = append(inputTensors, t)
		defer t.Destroy()
	}

	output, err := onnxruntime_go.NewEmptyTensor[float32](
		onnxruntime_go.Shape{int64(batch), int64(maxSeq), int64(o.dim)})
	if err != nil {
		return nil, fmt.Errorf("embedder: output tensor: %w", err)
	}
	defer output.Destroy()

	o.mu.Lock()
	err = o.session.Run(inputTensors, []onnxruntime_go.Value{output})
	o.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("embedder: run: %w", err)
	}

	shape := output.GetShape()
	data := output.GetData()
	switch len(shape) {
	case 3:
		return meanPool(data, shape, masks)
	case 2:
		return matrixToVector(data, shape)
	default:
		return nil, fmt.Errorf("embedder: unexpected output rank %d (shape %v)", len(shape), shape)
	}
}

// Dimensions implements Embedder.
func (o *ONNX) Dimensions() int { return o.dim }

// ModelVersion implements Embedder.
func (o *ONNX) ModelVersion() string { return o.version }

func flattenInt64(seqs [][]int64, width int) []int64 {
	out := make([]int64, 0, len(seqs)*width)
	for _, s := range seqs {
		for i := 0; i < width; i++ {
			if i < len(s) {
				out = append(out, s[i])
			} else {
				out = append(out, 0)
			}
		}
	}
	return out
}

// loadTokenizer reads the HuggingFace tokenizer.json: the WordPiece vocab and
// whether the model lowercases input.
func loadTokenizer(path string) (*wordPiece, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("embedder: read tokenizer: %w", err)
	}
	var doc struct {
		Normalizer struct {
			LowerCase bool `json:"lowercase"`
		} `json:"normalizer"`
		Model struct {
			Type  string           `json:"type"`
			Vocab map[string]int32 `json:"vocab"`
		} `json:"model"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("embedder: parse tokenizer.json: %w", err)
	}
	if doc.Model.Type != "WordPiece" || len(doc.Model.Vocab) == 0 {
		return nil, fmt.Errorf("embedder: tokenizer.json is not a WordPiece model (got %q)", doc.Model.Type)
	}
	return newWordPiece(doc.Model.Vocab, doc.Normalizer.LowerCase, 512), nil
}
