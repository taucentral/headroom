package headroom

import (
	"context"
	"regexp"
	"strings"

	tau "github.com/taucentral/tau/pkg/tau"
)

// OutputObserver is a ResponseObserver that drops ceremony from the
// persisted assistant response. It does NOT modify the bytes the model
// streamed to the embedder; it only rewrites the *Response value passed
// to ObserveResponse, which is the form tau persists.
type OutputObserver struct {
	// DropLeadingPhrases lists ceremony phrases stripped from the
	// start of the first TextContent block. Default: "Certainly,",
	// "Sure,", "Let me help with that.", "Of course,".
	DropLeadingPhrases []string

	// DropTrailingRestatement, when true, strips a trailing
	// restatement of the user's question from the response.
	DropTrailingRestatement bool

	// DropThinkingSections, when true, removes model-emitted
	// "thinking"/"analysis" blocks from the persisted response.
	// Sections are delimited by tag pairs in ThinkingTags (default
	// `thinking`, `analysis`, `reflection`). A section is only
	// removed when both its open and close tags are present; an
	// unterminated open tag is left intact so the embedder can
	// decide what to do with it. Opt-in (default false).
	DropThinkingSections bool

	// ThinkingTags lists the tag names treated as thinking-style
	// sections when DropThinkingSections is true. Each entry is
	// the bare tag name (e.g. "thinking"); the observer matches
	// `<thinking>...</thinking>` case-insensitively. Defaults are
	// applied at ObserveResponse time when this slice is nil.
	ThinkingTags []string
}

// defaultThinkingTags are the section tag names the observer strips
// when DropThinkingSections is true and ThinkingTags is nil.
var defaultThinkingTags = []string{"thinking", "analysis", "reflection"}

// NewOutputObserver returns an OutputObserver with default rules.
// Defaults: leading ceremony phrases dropped, trailing restatement
// dropped, thinking-section stripping OFF (opt-in per the spec's
// "(per config)" wording).
func NewOutputObserver() *OutputObserver {
	return &OutputObserver{
		DropLeadingPhrases: []string{
			"Certainly,",
			"Certainly!",
			"Sure,",
			"Sure!",
			"Let me help with that.",
			"Let me help with that",
			"Of course,",
			"Of course!",
		},
		DropTrailingRestatement: true,
	}
}

// ObserveResponse satisfies tau.ResponseObserver. The observer never
// returns a non-nil error unless ctx is cancelled. Any rewrite error
// leaves the response unchanged.
func (o *OutputObserver) ObserveResponse(ctx context.Context, req *tau.Request, resp *tau.Response, streamErr error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if resp == nil {
		return nil
	}
	for i := range resp.Content {
		tc, ok := resp.Content[i].(tau.TextContent)
		if !ok {
			continue
		}
		text := tc.Text
		text = o.dropLeading(text)
		if o.DropThinkingSections {
			text = o.dropThinkingSections(text)
		}
		if o.DropTrailingRestatement {
			text = o.dropTrailingRestatement(text)
		}
		resp.Content[i] = tau.TextContent{Text: text}
	}
	return nil
}

// dropLeading removes any configured leading ceremony phrase from
// text. A single phrase is removed per call; the function loops to
// catch multi-phrase openings.
func (o *OutputObserver) dropLeading(text string) string {
	for {
		trimmed := strings.TrimLeft(text, " \t\n\r")
		var matched bool
		for _, p := range o.DropLeadingPhrases {
			if strings.HasPrefix(trimmed, p) {
				after := strings.TrimSpace(trimmed[len(p):])
				text = after
				matched = true
				break
			}
		}
		if !matched {
			return text
		}
	}
}

// dropTrailingRestatement drops a trailing paragraph that begins with
// markers like "In summary," / "To summarize," / "Hope this helps" /
// "Let me know if". The cut is conservative: a single trailing block
// separated by a blank line is removed.
func (o *OutputObserver) dropTrailingRestatement(text string) string {
	markers := []string{
		"\n\nin summary,",
		"\n\nto summarize,",
		"\n\nto summarise,",
		"\n\nhope this helps",
		"\n\nlet me know if",
		"\n\nlet me know if you",
	}
	lower := strings.ToLower(text)
	idx := -1
	for _, m := range markers {
		if i := strings.Index(lower, m); i >= 0 {
			if idx == -1 || i < idx {
				idx = i
			}
		}
	}
	if idx < 0 {
		return text
	}
	return strings.TrimRight(text[:idx], " \t\r\n")
}

// dropThinkingSections removes every balanced `<tag>...</tag>` block
// where tag is in ThinkingTags (defaultThinkingTags if nil). The match
// is case-insensitive on the tag name and the close tag must match the
// open tag. Unbalanced opens are left intact. Adjacent whitespace
// collapsed by the surrounding rewrite pipeline may leave a blank
// line; we trim a single leading newline pair per removal to avoid
// piling up blanks across multiple sections.
func (o *OutputObserver) dropThinkingSections(text string) string {
	tags := o.ThinkingTags
	if len(tags) == 0 {
		tags = defaultThinkingTags
	}
	for _, tag := range tags {
		re := thinkingSectionRe(tag)
		text = re.ReplaceAllString(text, "")
	}
	// Collapse any blank-line runs introduced by the excisions; the
	// surrounding pipeline already tolerates a 2-newline cap, but we
	// avoid feeding it 4+ newlines from adjacent sections.
	return collapseAdjacentBlanks(text)
}

// thinkingSectionRe builds a case-insensitive regex for the given tag
// that matches `<tag>...</tag>` non-greedily across newlines. The
// close tag is required; a missing close tag is treated as
// "not a section" and left intact (conservative).
func thinkingSectionRe(tag string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(tag)
	return regexp.MustCompile(`(?is)<` + escaped + `>.*?</` + escaped + `>`)
}

// collapseAdjacentBlanks caps runs of 3+ newlines back to 2. It is a
// local helper for dropThinkingSections; the broader text pipeline
// has its own blank-line collapser.
func collapseAdjacentBlanks(text string) string {
	for {
		i := strings.Index(text, "\n\n\n")
		if i < 0 {
			return text
		}
		text = text[:i] + "\n\n" + text[i+3:]
	}
}
