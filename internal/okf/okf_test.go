package okf_test

import (
	"bytes"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name           string
		src            string
		hasFrontmatter bool
		wantBody       string
	}{
		{
			name:           "fence at byte 0 is frontmatter",
			src:            "---\ntitle: Hi\n---\n# Body\n",
			hasFrontmatter: true,
			wantBody:       "# Body\n",
		},
		{
			name:           "fence inside code block is not frontmatter",
			src:            "# Heading\n\n```\n---\nnot: frontmatter\n---\n```\n",
			hasFrontmatter: false,
			wantBody:       "# Heading\n\n```\n---\nnot: frontmatter\n---\n```\n",
		},
		{
			name:           "unterminated opening fence is body-only",
			src:            "---\ntitle: never closed\n# body keeps going\n",
			hasFrontmatter: false,
			wantBody:       "---\ntitle: never closed\n# body keeps going\n",
		},
		{
			name:           "no frontmatter plain markdown",
			src:            "# Plain\n\ntext\n",
			hasFrontmatter: false,
			wantBody:       "# Plain\n\ntext\n",
		},
		{
			name:           "four dashes is not a fence",
			src:            "----\ntitle: nope\n----\n",
			hasFrontmatter: false,
			wantBody:       "----\ntitle: nope\n----\n",
		},
		{
			name:           "crlf fence is recognized",
			src:            "---\r\ntitle: Hi\r\n---\r\nbody\r\n",
			hasFrontmatter: true,
			wantBody:       "body\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := okf.Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if doc.HasFrontmatter != tt.hasFrontmatter {
				t.Fatalf("HasFrontmatter = %v, want %v", doc.HasFrontmatter, tt.hasFrontmatter)
			}
			if !bytes.Equal(doc.Body, []byte(tt.wantBody)) {
				t.Fatalf("Body = %q, want %q", doc.Body, tt.wantBody)
			}
		})
	}
}

func TestParseEOLDetection(t *testing.T) {
	lf, _ := okf.Parse([]byte("# lf\nbody\n"))
	if lf.EOLStyle != okf.EOLLF {
		t.Fatalf("LF file detected as %v, want EOLLF", lf.EOLStyle)
	}
	crlf, _ := okf.Parse([]byte("# crlf\r\nbody\r\n"))
	if crlf.EOLStyle != okf.EOLCRLF {
		t.Fatalf("CRLF file detected as %v, want EOLCRLF", crlf.EOLStyle)
	}
}
