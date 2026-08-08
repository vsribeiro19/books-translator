package llm

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/vribeiro19/books-translator/api/orchestrator/internal/model"
)

// Line is a single translation unit inside a chunk. Blocks longer than the
// token budget are split into multiple lines that share the same BlockIdx.
type Line struct {
	ID       int
	Marker   string
	BlockIdx int
	Text     string
}

// Chunk is a group of consecutive lines sent to the LLM in one request.
type Chunk struct {
	Lines []Line
}

// Translator translates whole chapters while preserving block structure.
// Block boundaries are kept by replying to a numbered-segment protocol; long
// paragraphs are split across lines inside a chunk and merged afterwards.
type Translator struct {
	client        *Client
	chunkTokens   int
	contextChunks int
}

// NewTranslator builds a chapter translator backed by a chat client.
func NewTranslator(c *Client, chunkTokens, contextChunks int) *Translator {
	return &Translator{client: c, chunkTokens: chunkTokens, contextChunks: contextChunks}
}

var idLineRe = regexp.MustCompile(`^\s*(\d+)\s*\.\s*(.*)$`)

func estimateTokens(text string) int {
	return len(text)/4 + 1
}

// splitText splits a block text into lines respecting an estimated token budget.
func splitText(text string, startID *int, budget int) []Line {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []Line
	buf := make([]string, 0, 16)
	bufChars := 0
	for _, w := range words {
		bufChars += len(w) + 1
		if bufChars/4 >= budget && len(buf) > 0 {
			*startID++
			lines = append(lines, Line{ID: *startID, Text: strings.Join(buf, " ")})
			buf = buf[:0]
			bufChars = len(w) + 1
		}
		buf = append(buf, w)
	}
	if len(buf) > 0 {
		*startID++
		lines = append(lines, Line{ID: *startID, Text: strings.Join(buf, " ")})
	}
	return lines
}

// Plan splits a chapter into chunks. Line IDs are globally unique within the
// chapter so translated context can reference prior lines without collisions.
func (t *Translator) Plan(chapter model.Chapter) []*Chunk {
	var chunks []*Chunk
	var current []Line
	currentChars := 0
	globalID := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, &Chunk{Lines: append([]Line(nil), current...)})
		current = nil
		currentChars = 0
	}

	for bi, block := range chapter.Blocks {
		lines := splitText(block.Text, &globalID, t.chunkTokens)
		for i := range lines {
			lines[i].BlockIdx = bi
			lines[i].Marker = "P"
			if block.Type == "heading" {
				lines[i].Marker = "H"
			}
			size := estimateTokens(lines[i].Text)
			if len(current) > 0 && currentChars+size > t.chunkTokens {
				flush()
			}
			current = append(current, lines[i])
			currentChars += size
		}
	}
	flush()
	return chunks
}

// TranslateChapter translates a chapter end to end, keeping a rolling context
// of the previous contextChunks worth of translated lines.
func (t *Translator) TranslateChapter(ctx context.Context, chapter model.Chapter) ([]model.Block, error) {
	chunks := t.Plan(chapter)
	resultLines := make(map[int]Line, 0)

	contextLines := make([]Line, 0, 64)

	for i, chunk := range chunks {
		lines, err := t.translateLines(ctx, chunk.Lines, contextLines)
		if err != nil {
			return nil, fmt.Errorf("translate chunk %d/%d: %w", i+1, len(chunks), err)
		}
		for _, l := range lines {
			resultLines[l.ID] = l
		}
		contextLines = append(contextLines, lines...)
		contextLines = trimContext(contextLines, t.contextChunks, t.chunkTokens)
	}

	return assembleBlocks(chapter.Blocks, resultLines), nil
}

func trimContext(lines []Line, contextChunks, chunkTokens int) []Line {
	kept := make([]Line, 0, 16)
	chars := 0
	for i := len(lines) - 1; i >= 0; i-- {
		c := estimateTokens(lines[i].Text)
		if len(kept) > 0 && chars+c > contextChunks*chunkTokens {
			break
		}
		kept = append(kept, lines[i])
		chars += c
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return kept
}

func (t *Translator) translateLines(ctx context.Context, lines []Line, contextLines []Line) ([]Line, error) {
	var sb strings.Builder
	sb.WriteString("Translate the numbered segments below to Brazilian Portuguese.\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Translate all segments faithfully; keep numbers, names and technical terms appropriate.\n")
	sb.WriteString("- Start each translated segment with its original number, e.g. \"3. <translated text>\".\n")
	sb.WriteString("- Return each segment on its own line, in the same order, and nothing else.\n")

	if len(contextLines) > 0 {
		sb.WriteString("\nAlready-translated context (for coherence only; do not repeat it):\n")
		from := 0
		if len(contextLines) > 40 {
			from = len(contextLines) - 40
		}
		for _, l := range contextLines[from:] {
			fmt.Fprintf(&sb, "%d. %s\n", l.ID, l.Text)
		}
	}

	sb.WriteString("\nSegments to translate:\n")
	for _, l := range lines {
		fmt.Fprintf(&sb, "%d. %s\n", l.ID, l.Text)
	}

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: sb.String()},
	}

	content, err := t.client.complete(ctx, messages)
	if err != nil {
		return nil, err
	}

	return parseResponse(content, lines)
}

const systemPrompt = "You are a professional translator of literary and academic content. " +
	"Translate accurately from English to Brazilian Portuguese, preserving meaning, tone and structure."

// parseResponse converts the model's numbered reply back into Lines.
func parseResponse(content string, expected []Line) ([]Line, error) {
	ids := make(map[int]bool)
	for _, l := range expected {
		ids[l.ID] = true
	}

	out := make([]Line, 0, len(expected))
	var cur *Line
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimRight(raw, "\r")
		m := idLineRe.FindStringSubmatch(line)
		if m != nil {
			id, err := strconv.Atoi(m[1])
			if err != nil || !ids[id] {
				continue
			}
			translated := strings.TrimSpace(m[2])
			out = append(out, Line{ID: id, Text: translated})
			cur = &out[len(out)-1]
			continue
		}
		text := strings.TrimSpace(line)
		if text == "" || cur == nil {
			continue
		}
		cur.Text = strings.TrimSpace(cur.Text + " " + text)
	}

	found := make(map[int]bool)
	for _, l := range out {
		found[l.ID] = true
	}
	if len(found) != len(ids) {
		missing := 0
		for id := range ids {
			if !found[id] {
				missing++
			}
		}
		return nil, fmt.Errorf("response missing %d translated segment(s)", missing)
	}

	seenID := map[int]Line{}
	for _, l := range out {
		if _, ok := found[l.ID]; ok {
			seenID[l.ID] = l
		}
	}

	result := make([]Line, 0, len(seenID))
	for _, l := range expected {
		key := l.ID
		if got, ok := seenID[key]; ok {
			got.BlockIdx = l.BlockIdx
			got.Marker = l.Marker
			result = append(result, got)
		}
	}
	return result, nil
}

// assembleBlocks rebuilds chapter blocks from translated lines, merging split
// paragraphs back into single blocks in the original order.
func assembleBlocks(original []model.Block, translated map[int]Line) []model.Block {
	out := make([]model.Block, 0, len(original))
	for bi, block := range original {
		var parts []string
		for _, l := range translated {
			if l.BlockIdx == bi {
				parts = append(parts, l.Text)
			}
		}
		text := strings.Join(parts, " ")
		if text == "" {
			text = block.Text
		}
		out = append(out, model.Block{Type: block.Type, Level: block.Level, Text: text})
	}
	return out
}
