package graph

import (
	"reflect"
	"sort"
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

// existsSet builds an existence check over a fixed set of repo-relative paths,
// mirroring repo.Exists's (bool, error) signature.
func existsSet(paths ...string) func(string) (bool, error) {
	set := map[string]struct{}{}
	for _, p := range paths {
		set[p] = struct{}{}
	}
	return func(p string) (bool, error) {
		_, ok := set[p]
		return ok, nil
	}
}

func TestExtract_OutboundLinks(t *testing.T) {
	body := []byte(`See [B](b.md) and [sub](sub/c.md#heading) and [ext](https://example.com/x)
and [abs](/root.md) and [mail](mailto:a@b.com) and [dup](b.md) and [self](a.md)
and [dangling](nope.md) and [notmd](image.png)

` + "```" + `
this [fenced](should-not-count.md) link is in a code block
` + "```" + `

inline ` + "`" + `[code](alsno-not.md)` + "`" + ` span
`)
	exists := existsSet("b.md", "sub/c.md", "a.md")
	got, err := outboundLinks("a.md", body, exists)
	if err != nil {
		t.Fatalf("outboundLinks: %v", err)
	}
	sort.Strings(got)
	want := []string{"b.md", "sub/c.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outboundLinks = %v, want %v", got, want)
	}
}

func TestExtract_OutboundLinks_CodeBlockSkipped(t *testing.T) {
	// A link that appears ONLY inside a fenced code block must NOT become an edge
	// (inherited from okf.FindLinks skipping fenced spans).
	body := []byte("intro\n\n```\n[hidden](target.md)\n```\n\noutro\n")
	exists := existsSet("target.md")
	got, err := outboundLinks("a.md", body, exists)
	if err != nil {
		t.Fatalf("outboundLinks: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no edges from a code-block-only link, got %v", got)
	}
}

func TestExtract_OutboundLinks_RelativeResolution(t *testing.T) {
	// A link from a nested page resolves relative to the page's directory, matching
	// okf.RewriteLinks resolution (path.Clean(path.Join(dir, dest))).
	body := []byte("[up](../top.md) and [sibling](other.md)\n")
	exists := existsSet("top.md", "docs/other.md")
	got, err := outboundLinks("docs/page.md", body, exists)
	if err != nil {
		t.Fatalf("outboundLinks: %v", err)
	}
	sort.Strings(got)
	want := []string{"docs/other.md", "top.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outboundLinks = %v, want %v", got, want)
	}
}

func TestExtract_PageTags(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "flow sequence",
			src:  "---\ntitle: A\ntags: [a, b]\n---\nbody\n",
			want: []string{"a", "b"},
		},
		{
			name: "block list",
			src:  "---\ntitle: A\ntags:\n  - ops\n  - infra\n---\nbody\n",
			want: []string{"ops", "infra"},
		},
		{
			name: "single scalar",
			src:  "---\ntitle: A\ntags: ops\n---\nbody\n",
			want: []string{"ops"},
		},
		{
			name: "no tags key",
			src:  "---\ntitle: A\n---\nbody\n",
			want: nil,
		},
		{
			name: "empty flow sequence",
			src:  "---\ntitle: A\ntags: []\n---\nbody\n",
			want: []string{},
		},
		{
			name: "blank items skipped",
			src:  "---\ntitle: A\ntags:\n  - ops\n  - \"\"\n---\nbody\n",
			want: []string{"ops"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := okf.Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("okf.Parse: %v", err)
			}
			got := pageTags(doc)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("pageTags = %#v, want %#v", got, tc.want)
			}
		})
	}
}
