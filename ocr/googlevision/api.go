package googlevision

import (
	"encoding/json"
	"fmt"
	"image"

	"github.com/reynaldio/id-ocr/ocr"
)

// --- request shapes (images:annotate) ---

type annotateRequest struct {
	Requests []annotateImageRequest `json:"requests"`
}

type annotateImageRequest struct {
	Image        imageData     `json:"image"`
	Features     []feature     `json:"features"`
	ImageContext *imageContext `json:"imageContext,omitempty"`
}

type imageData struct {
	Content string `json:"content"`
}

type feature struct {
	Type string `json:"type"`
}

type imageContext struct {
	LanguageHints []string `json:"languageHints,omitempty"`
}

// --- response shapes ---

type annotateResponse struct {
	Responses []struct {
		FullTextAnnotation struct {
			Text string `json:"text"`
		} `json:"fullTextAnnotation"`
		TextAnnotations []entityAnnotation `json:"textAnnotations"`
		Error           *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"responses"`
}

type entityAnnotation struct {
	Description  string `json:"description"`
	BoundingPoly struct {
		Vertices []struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"vertices"`
	} `json:"boundingPoly"`
}

// parseResult turns a raw images:annotate response into an ocr.Result. The full
// text comes from fullTextAnnotation; per-word boxes come from the individual
// textAnnotations entries (index 0 is the whole text, so words start at 1).
func parseResult(raw []byte) (*ocr.Result, error) {
	var resp annotateResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("googlevision: decode response: %w", err)
	}
	if len(resp.Responses) == 0 {
		return &ocr.Result{}, nil
	}
	r := resp.Responses[0]
	if r.Error != nil {
		return nil, fmt.Errorf("googlevision: API error %d: %s", r.Error.Code, r.Error.Message)
	}

	result := &ocr.Result{}
	for _, a := range wordAnnotations(r.TextAnnotations) {
		result.Words = append(result.Words, ocr.Word{
			Text: a.Description,
			Box:  boundingRect(a),
		})
	}

	// Prefer text reconstructed from word geometry: Vision serialises form
	// layouts (like a KTP) with the label and value columns interleaved, which
	// breaks line-based parsing. Fall back to the API's own text when no word
	// boxes are available.
	result.Text = ocr.ReconstructText(result.Words)
	if result.Text == "" {
		result.Text = r.FullTextAnnotation.Text
		if result.Text == "" && len(r.TextAnnotations) > 0 {
			result.Text = r.TextAnnotations[0].Description
		}
	}
	return result, nil
}

func wordAnnotations(all []entityAnnotation) []entityAnnotation {
	if len(all) <= 1 {
		return nil
	}
	return all[1:]
}

func boundingRect(a entityAnnotation) image.Rectangle {
	v := a.BoundingPoly.Vertices
	if len(v) == 0 {
		return image.Rectangle{}
	}
	minX, minY := v[0].X, v[0].Y
	maxX, maxY := v[0].X, v[0].Y
	for _, p := range v[1:] {
		minX = min(minX, p.X)
		minY = min(minY, p.Y)
		maxX = max(maxX, p.X)
		maxY = max(maxY, p.Y)
	}
	return image.Rect(minX, minY, maxX, maxY)
}
