package tensor

import (
	"image"
	"sort"

	"github.com/pkg/errors"
	tf "github.com/tensorflow/tensorflow/tensorflow/go"
)

type Model struct {
	model                   *tf.SavedModel
	labels                  Labels
	inputLayer, outputLayer string
	imageMode               string
}

func NewModel(modelName, tagName, inputLayer, outputLayer, imageMode, labels string) (*Model, func() error, error) {
	model, err := tf.LoadSavedModel(modelName, []string{tagName}, nil)
	if err != nil {
		return nil, nil, errors.Wrapf(err,
			"failed to load saved model %q / tag %q", modelName, tagName)
	}

	m := &Model{
		model:       model,
		inputLayer:  inputLayer,
		outputLayer: outputLayer,
		imageMode:   imageMode,
	}

	if labels != "" {
		l, err := labelsFromFile(labels)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to read labels from file %q", labels)
		}
		m.labels = l
	}

	return m, model.Session.Close, nil
}

type Predictions struct {
	scores []float32
	labels Labels
}

type Prediction struct {
	Label string
	Score float32
}

func (p *Predictions) Best(n int) []Prediction {
	var ret []Prediction

	for i, score := range p.scores {
		ret = append(ret, Prediction{
			Label: p.labels.Get(i),
			Score: score,
		})
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Score > ret[j].Score
	})

	return ret[:n]
}

func (m *Model) Inception(img image.Image) (*Predictions, error) {
	tensor, err := imageToTensor(img, m.imageMode)
	if err != nil {
		return nil, errors.Wrap(err, "error converting image to tensor")
	}

	result, err := m.model.Session.Run(
		map[tf.Output]*tf.Tensor{
			m.model.Graph.Operation(m.inputLayer).Output(0): tensor,
		},
		[]tf.Output{
			m.model.Graph.Operation(m.outputLayer).Output(0),
		},
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "error running the model session")
	}

	if len(result) < 1 {
		return nil, errors.New("result is empty")
	}

	res, ok := result[0].Value().([][]float32)
	if !ok {
		return nil, errors.Errorf("result has unexpected type %T", result[0].Value())
	}

	if len(res) < 1 {
		return nil, errors.New("predictions are empty")
	}

	return &Predictions{
		scores: res[0],
		labels: m.labels}, nil
}

const (
	ImageModeTensorflow = "tf"
	ImageModeCaffe      = "caffe"
)

func imageToTensor(img image.Image, imageMode string) (*tf.Tensor, error) {
	switch imageMode {
	case ImageModeTensorflow:
		return imageToTensorTF(img)
	case ImageModeCaffe:
		return imageToTensorCaffe(img)
	}

	return nil, errors.Errorf("unknown image mode %q", imageMode)
}

func imageToTensorTF(img image.Image) (*tf.Tensor, error) {
	var image [1][299][299][3]float32
	for i := 0; i < 299; i++ {
		for j := 0; j < 299; j++ {
			r, g, b, _ := img.At(i, j).RGBA()
			image[0][i][j][0] = convertTF(r)
			image[0][i][j][1] = convertTF(g)
			image[0][i][j][2] = convertTF(b)
		}
	}

	return tf.NewTensor(image)
}

func convertTF(value uint32) float32 {
	return (float32(value>>8) - float32(127.5)) / float32(127.5)
}

func imageToTensorCaffe(img image.Image) (*tf.Tensor, error) {
	var image [1][224][224][3]float32
	for i := 0; i < 224; i++ {
		for j := 0; j < 224; j++ {
			r, g, b, _ := img.At(i, j).RGBA()
			image[0][i][j][0] = convertCaffe(b) - 103.939
			image[0][i][j][1] = convertCaffe(g) - 116.779
			image[0][i][j][2] = convertCaffe(r) - 123.68
		}
	}

	return tf.NewTensor(image)
}

func convertCaffe(value uint32) float32 {
	return float32(value >> 8)
}
