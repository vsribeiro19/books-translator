package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vribeiro19/books-translator/api/orchestrator/internal/model"
)

func TestSplitTextHonorsBudget(t *testing.T) {
	start := 0
	long := strings.Repeat("lorem ipsum dolor sit amet ", 50)
	lines := splitText(long, &start, 40)
	if len(lines) < 2 {
		t.Fatalf("expected long text split into multiple lines, got %d", len(lines))
	}
	for i, l := range lines {
		if l.ID != i+1 {
			t.Fatalf("expected sequential ids, line %d has id %d", i, l.ID)
		}
		if estimateTokens(l.Text) > 50 {
			t.Fatalf("line %d over budget: %d tokens", i, estimateTokens(l.Text))
		}
	}
}

func TestPlanChunksByBudget(t *testing.T) {
	tr := NewTranslator(nil, 40, 2)
	texts := make([]string, 10)
	for i := range texts {
		texts[i] = strings.Repeat("word ", 30) + "."
	}
	blocks := make([]model.Block, len(texts))
	for i, txt := range texts {
		blocks[i] = model.Block{Type: "paragraph", Level: 0, Text: txt}
	}
	chunks := tr.Plan(model.Chapter{Title: "C1", Blocks: blocks})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	// All block indices represented exactly once.
	seen := map[int]bool{}
	for _, c := range chunks {
		for _, l := range c.Lines {
			if seen[l.BlockIdx] {
				t.Fatalf("block %d repeated across chunks", l.BlockIdx)
			}
			seen[l.BlockIdx] = true
		}
	}
	for i := range blocks {
		if !seen[i] {
			t.Fatalf("block %d missing", i)
		}
	}
}

func TestParseResponseRoundTrip(t *testing.T) {
	expected := []Line{
		{ID: 1, BlockIdx: 0, Marker: "P", Text: "first paragraph source"},
		{ID: 2, BlockIdx: 0, Marker: "P", Text: "continued line"},
		{ID: 3, BlockIdx: 1, Marker: "H", Text: "heading source"},
	}
	reply := "1. Primeiro paragrafo traduzido\ncontinuando o texto.\n2. linha seguinte\n3. Titulo traduzido"

	got, err := parseResponse(reply, expected)
	if err != nil {
		t.Fatalf("parseResponse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}
	if got[0].ID != 1 || !strings.Contains(got[0].Text, "continuando") {
		t.Fatalf("continuation not merged: %+v", got[0])
	}
	if got[2].Marker != "H" {
		t.Fatalf("expected heading marker, got %q", got[2].Marker)
	}
}

func TestParseResponseMissingSegments(t *testing.T) {
	expected := []Line{{ID: 1, Text: "a"}, {ID: 2, Text: "b"}}
	if _, err := parseResponse("1. so um segmento", expected); err == nil {
		t.Fatal("expected error for missing segment")
	}
}

func TestAssembleBlocksMergesSplitLines(t *testing.T) {
	original := []model.Block{
		{Type: "paragraph", Level: 0, Text: "a b c d"},
		{Type: "heading", Level: 1, Text: "Title"},
	}
	translated := map[int]Line{
		1: {ID: 1, BlockIdx: 0, Text: "A B"},
		2: {ID: 2, BlockIdx: 0, Text: "C D"},
		3: {ID: 3, BlockIdx: 1, Text: "Titulo"},
	}
	out := assembleBlocks(original, translated)
	if out[0].Text != "A B C D" {
		t.Fatalf("expected merged paragraph, got %q", out[0].Text)
	}
	if out[1].Text != "Titulo" || out[1].Type != "heading" || out[1].Level != 1 {
		t.Fatalf("unexpected heading block: %+v", out[1])
	}
}

func TestTranslateChapterWithFakeClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		out := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "1. Bom dia\n2. Adeus"}},
			},
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	client := New(srv.URL, "key", "m", time.Hour, 0, 0)
	tr := NewTranslator(client, 1000, 2)
	chapter := model.Chapter{
		Title: "C1",
		Blocks: []model.Block{
			{Type: "paragraph", Level: 0, Text: "Good morning."},
			{Type: "paragraph", Level: 0, Text: "Goodbye."},
		},
	}
	got, err := tr.TranslateChapter(context.Background(), chapter)
	if err != nil {
		t.Fatalf("TranslateChapter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got))
	}
	if got[0].Text != "Bom dia" || got[1].Text != "Adeus" {
		t.Fatalf("unexpected translations: %+v", got)
	}
}
