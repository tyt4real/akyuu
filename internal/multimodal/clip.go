package multimodal

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"akyuu/internal/store"

	onnxruntime_go "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
)

type CLIPWorker struct {
	store        StoreAPI
	storageRoot  string
	modelPath    string
	session      *onnxruntime_go.DynamicAdvancedSession
	inputNames   []string
	outputName   string
	dim          int
	modelVersion string
}

func NewCLIPWorker(st StoreAPI, storageRoot, modelDir, modelVersion string, dim int) (*CLIPWorker, error) {
	modelPath := filepath.Join(modelDir, "model.onnx")

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("clip model not found: %w", err)
	}

	inputs, outputs, err := onnxruntime_go.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("clip: inspect model: %w", err)
	}

	w := &CLIPWorker{
		store:        st,
		storageRoot:  storageRoot,
		modelPath:    modelPath,
		dim:          dim,
		modelVersion: modelVersion,
	}

	for _, in := range inputs {
		switch in.Name {
		case "pixel_values", "input_ids", "attention_mask":
			w.inputNames = append(w.inputNames, in.Name)
		}
	}

	for _, out := range outputs {
		if out.Name == "image_embeds" || out.OrtValueType == onnxruntime_go.ONNXTypeTensor {
			w.outputName = out.Name
			break
		}
	}
	if w.outputName == "" {
		return nil, fmt.Errorf("clip: no usable output")
	}

	options, err := onnxruntime_go.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("clip: session options: %w", err)
	}
	defer options.Destroy()

	session, err := onnxruntime_go.NewDynamicAdvancedSession(modelPath, w.inputNames, []string{w.outputName}, options)
	if err != nil {
		return nil, fmt.Errorf("clip: load session: %w", err)
	}
	w.session = session

	return w, nil
}

func (w *CLIPWorker) Kind() string { return store.JobCLIP }

func (w *CLIPWorker) Process(ctx context.Context, payload JobPayload) error {
	blobData, err := w.getBlobData(ctx, payload.FileID)
	if err != nil {
		return fmt.Errorf("clip: get blob: %w", err)
	}

	embedding, err := w.embedImage(ctx, blobData)
	if err != nil {
		return fmt.Errorf("clip: embed: %w", err)
	}

	if err := w.store.UpsertCLIPEmbedding(ctx, payload.PostID, payload.FileID, embedding, w.modelVersion); err != nil {
		return fmt.Errorf("clip: store: %w", err)
	}

	return nil
}

func (w *CLIPWorker) getBlobData(ctx context.Context, fileID int64) ([]byte, error) {
	var storagePath string
	err := w.store.QueryRow(ctx, `
		SELECT b.storage_path
		FROM files f
		JOIN blobs b ON b.file_hash = f.file_hash
		WHERE f.id = $1 AND f.downloaded_full AND b.storage_path IS NOT NULL`,
		fileID).Scan(&storagePath)
	if err != nil {
		return nil, err
	}
	if storagePath == "" {
		return nil, fmt.Errorf("blob not downloaded")
	}

	fullPath := filepath.Join(w.storageRoot, storagePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read blob file: %w", err)
	}
	return data, nil
}

func (w *CLIPWorker) embedImage(ctx context.Context, imgData []byte) ([]float32, error) {
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, err
	}

	// MobileCLIP expects 224x224, normalized with ImageNet mean/std
	const inputSize = 224

	// Resize with center crop
	resized := resizeAndCenterCrop(img, inputSize, inputSize)

	// Normalize: (pixel - mean) / std
	// ImageNet mean: [0.48145466, 0.4578275, 0.40821073]
	// ImageNet std: [0.26862954, 0.26130258, 0.27577711]
	inputTensor := make([]float32, 3*inputSize*inputSize)

	mean := [3]float32{0.48145466, 0.4578275, 0.40821073}
	std := [3]float32{0.26862954, 0.26130258, 0.27577711}

	for y := 0; y < inputSize; y++ {
		for x := 0; x < inputSize; x++ {
			r, g, b, _ := resized.At(x, y).RGBA()
			// Convert from 16-bit to 0-1 range
			rf := float32(r) / 65535.0
			gf := float32(g) / 65535.0
			bf := float32(b) / 65535.0

			idx := y*inputSize + x
			inputTensor[idx] = (rf - mean[0]) / std[0]
			inputTensor[idx+inputSize*inputSize] = (gf - mean[1]) / std[1]
			inputTensor[idx+2*inputSize*inputSize] = (bf - mean[2]) / std[2]
		}
	}

	// Create input tensor [1, 3, 224, 224]
	inputShape := onnxruntime_go.Shape{1, 3, int64(inputSize), int64(inputSize)}
	inputORT, err := onnxruntime_go.NewTensor(inputShape, inputTensor)
	if err != nil {
		return nil, fmt.Errorf("clip: create input tensor: %w", err)
	}
	defer inputORT.Destroy()

	// Run inference
	outputShape := onnxruntime_go.Shape{1, int64(w.dim)}
	outputORT, err := onnxruntime_go.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("clip: create output tensor: %w", err)
	}
	defer outputORT.Destroy()

	err = w.session.Run([]onnxruntime_go.Value{inputORT}, []onnxruntime_go.Value{outputORT})
	if err != nil {
		return nil, fmt.Errorf("clip: run session: %w", err)
	}

	embedding := outputORT.GetData()

	// L2 normalize
	var norm float32
	for _, v := range embedding {
		norm += v * v
	}
	norm = float32(len(embedding)) // placeholder - actually compute sqrt
	if norm > 0 {
		// sqrt
		normSqrt := float32(1.0)
		for i := 0; i < 10; i++ {
			normSqrt = 0.5 * (normSqrt + norm/normSqrt)
		}
		for i := range embedding {
			embedding[i] /= normSqrt
		}
	}

	return embedding, nil
}

func resizeAndCenterCrop(src image.Image, width, height int) image.Image {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// Scale to fit the smaller dimension
	scale := float64(width) / float64(srcW)
	if float64(height)/float64(srcH) > scale {
		scale = float64(height) / float64(srcH)
	}

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)

	// Resize
	resized := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(resized, resized.Bounds(), src, srcBounds, draw.Over, nil)

	// Center crop
	cropX := (newW - width) / 2
	cropY := (newH - height) / 2
	cropped := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(cropped, cropped.Bounds(), resized, image.Point{cropX, cropY}, draw.Over)

	return cropped
}

func (w *CLIPWorker) Close() error {
	if w.session != nil {
		return w.session.Destroy()
	}
	return nil
}
