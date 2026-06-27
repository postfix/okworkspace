package okf_test

import (
	"testing"

	"github.com/postfix/okworkspace/internal/okf"
)

func TestValidateTagCharset_ValidTags(t *testing.T) {
	valid := []string{
		"hello",
		"world123",
		"my-tag",
		"under_score",
		"tag with spaces",
		"A",
		"1",
		"-",
		"_",
		"MixedCase",
		"a-b_c d",
	}
	for _, tag := range valid {
		if err := okf.ValidateTagCharset(tag); err != nil {
			t.Errorf("ValidateTagCharset(%q) = %v, want nil", tag, err)
		}
	}
}

func TestValidateTagCharset_InvalidTags(t *testing.T) {
	invalid := []struct {
		tag     string
		reason  string
	}{
		{"<script>", "XSS script tag"},
		{"<img src=x onerror=alert(1)>", "XSS img tag"},
		{"onclick=alert(1)", "event handler"},
		{"javascript:alert(1)", "javascript scheme"},
		{"hello@world", "email address"},
		{"tag/with/slashes", "slashes"},
		{"tag.with.dots", "dots"},
		{"tag%20encoded", "URL encoding"},
		{"tag\\with\\backslashes", "backslashes"},
		{"tag\nnewline", "newline"},
		{"tab\tinside", "tab"},
		{"quote's", "single quote"},
		{`quote"s`, "double quote"},
		{"emoji😀", "emoji"},
		{"café", "accented characters"},
		{"<a href=\"javascript:void(0)\">", "anchor XSS"},
		{"你好", "CJK characters"},
	}
	for _, tc := range invalid {
		if err := okf.ValidateTagCharset(tc.tag); err == nil {
			t.Errorf("ValidateTagCharset(%q) = nil, want error (reason: %s)", tc.tag, tc.reason)
		}
	}
}

// TestValidateTagCharset_EmptyTag verifies an empty tag is rejected (zero-length
// doesn't match +).
func TestValidateTagCharset_EmptyTag(t *testing.T) {
	if err := okf.ValidateTagCharset(""); err == nil {
		t.Error("ValidateTagCharset(\"\") = nil, want error")
	}
}

// TestSetTags_RejectsInvalidChars in SetTags validates charset defense-in-depth.
func TestSetTags_RejectsInvalidCharset(t *testing.T) {
	src := []byte("---\ntitle: Test\n---\n\nbody\n")
	doc, err := okf.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := okf.SetTags(doc, []string{"<script>"}); err == nil {
		t.Error("SetTags with <script> tag = nil, want error")
	}
	if err := okf.SetTags(doc, []string{"valid-tag", "<img>"}); err == nil {
		t.Error("SetTags with mixed valid + invalid tag = nil, want error")
	}
}
