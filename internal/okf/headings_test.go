package okf

import (
	"reflect"
	"testing"
)

// TestScanHeadings_Levels: ATX headings of levels 1-6 are detected with their
// text and level; lines that are not headings are ignored.
func TestScanHeadings_Levels(t *testing.T) {
	body := []byte("# One\n\nintro text\n\n## Two\n\n### Three\n#### Four\n##### Five\n###### Six\n\nnot a heading\n####### Seven (7 hashes, not a heading)\n#NoSpace (not a heading)\n")
	got := ScanHeadings(body)
	want := []Heading{
		{Level: 1, Text: "One", Anchor: "#one"},
		{Level: 2, Text: "Two", Anchor: "#two"},
		{Level: 3, Text: "Three", Anchor: "#three"},
		{Level: 4, Text: "Four", Anchor: "#four"},
		{Level: 5, Text: "Five", Anchor: "#five"},
		{Level: 6, Text: "Six", Anchor: "#six"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScanHeadings levels mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// TestScanHeadings_SkipsFence: a `# ` line INSIDE a fenced code block is not a
// heading (both ``` and ~~~ fences).
func TestScanHeadings_SkipsFence(t *testing.T) {
	body := []byte("# Real Heading\n\n```\n# Not A Heading In Backtick Fence\n```\n\n~~~\n## Not A Heading In Tilde Fence\n~~~\n\n## After Fence\n")
	got := ScanHeadings(body)
	want := []Heading{
		{Level: 1, Text: "Real Heading", Anchor: "#real-heading"},
		{Level: 2, Text: "After Fence", Anchor: "#after-fence"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScanHeadings fence-skip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// TestScanHeadings_Slug: anchors are GitHub-style (lowercase, punctuation
// stripped, spaces→'-', NOT collapsed, underscore kept) — must mirror
// github-slugger so the rendered ids match.
func TestScanHeadings_Slug(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"My Section!", "#my-section"},
		{"Hello World", "#hello-world"},
		{"Hello   World", "#hello---world"}, // each space → '-' (no collapse)
		{"foo_bar", "#foo_bar"},             // underscore preserved
		{"C++ Tips", "#c-tips"},
		{"100% Done", "#100-done"},
	}
	for _, c := range cases {
		got := ScanHeadings([]byte("# " + c.text + "\n"))
		if len(got) != 1 {
			t.Fatalf("text %q: expected 1 heading, got %d", c.text, len(got))
		}
		if got[0].Anchor != c.want {
			t.Fatalf("slug for %q = %q, want %q", c.text, got[0].Anchor, c.want)
		}
	}
}

// TestScanHeadings_DuplicateSlug: duplicate headings get a numeric suffix
// (-1, -2) matching github-slugger's de-dup.
func TestScanHeadings_DuplicateSlug(t *testing.T) {
	body := []byte("# My Section\n\n## My Section\n\n### My Section\n")
	got := ScanHeadings(body)
	want := []string{"#my-section", "#my-section-1", "#my-section-2"}
	if len(got) != len(want) {
		t.Fatalf("expected %d headings, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i].Anchor != want[i] {
			t.Fatalf("dup slug[%d] = %q, want %q", i, got[i].Anchor, want[i])
		}
	}
}

// TestScanHeadings_Trim: trailing '#' closers and surrounding whitespace are
// trimmed from the heading text before slugging.
func TestScanHeadings_Trim(t *testing.T) {
	body := []byte("#    Spaced Title    \n\n##   Closed Title   ##   \n\n###\tTabbed\t###\n")
	got := ScanHeadings(body)
	want := []Heading{
		{Level: 1, Text: "Spaced Title", Anchor: "#spaced-title"},
		{Level: 2, Text: "Closed Title", Anchor: "#closed-title"},
		{Level: 3, Text: "Tabbed", Anchor: "#tabbed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScanHeadings trim mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}
