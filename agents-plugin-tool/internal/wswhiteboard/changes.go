package wswhiteboard

import (
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// A topic body edit leaves change marks: which blocks of the new body the
// edit added or rewrote, and where it deleted one. The page draws them as thin
// bars beside the body (assets/chg.js). Marks are cosmetic and best effort:
// blocks are matched by their text, so an occasional wrong pairing is
// acceptable, and nothing here may fail an update.

// Mark kinds.
const (
	MarkNew = "new" // a block the edit added
	MarkMod = "mod" // a block the edit rewrote
	MarkDel = "del" // a deletion point, just before the block (Block == block count: after the last)
)

// BlockMark marks block Block of a topic body, counted in bodyUnits order.
type BlockMark struct {
	Block int    `json:"b"`
	Kind  string `json:"k"`
}

// maxDiffCells bounds the block diff's work: a body pair past it gets no
// marks rather than a slow update.
const maxDiffCells = 1 << 20

// bodyUnit is one markable block of a parsed body.
type bodyUnit struct {
	node ast.Node
	// key identifies the block's content; equal keys match across edits.
	key string
}

// bodyUnits lists a body's markable blocks in document order: every leaf
// block (paragraph, heading, table, fenced or indented code, HTML block,
// thematic break), every list item for its own content (not its nested
// lists, whose items are units of their own), and the children of a
// blockquote.
func bodyUnits(doc ast.Node, src []byte) []bodyUnit {
	var out []bodyUnit
	collectUnits(doc, src, &out)
	return out
}

func collectUnits(parent ast.Node, src []byte, out *[]bodyUnit) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.Kind() {
		case ast.KindList, ast.KindBlockquote:
			collectUnits(c, src, out)
		case ast.KindListItem:
			*out = append(*out, bodyUnit{c, unitKey(c, src)})
			for sub := c.FirstChild(); sub != nil; sub = sub.NextSibling() {
				if sub.Kind() == ast.KindList {
					collectUnits(sub, src, out)
				}
			}
		default:
			*out = append(*out, bodyUnit{c, unitKey(c, src)})
		}
	}
}

// unitKey is a block's kind plus its whitespace-normalized text: inline text,
// code and HTML lines, a fence's language, and link destinations. A list
// item's key leaves out its nested lists.
func unitKey(root ast.Node, src []byte) string {
	var b strings.Builder
	b.WriteString(root.Kind().String() + " ")
	if h, ok := root.(*ast.Heading); ok {
		b.WriteString(strconv.Itoa(h.Level))
	}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n != root && n.Kind() == ast.KindList {
			return ast.WalkSkipChildren, nil
		}
		if n.Type() == ast.TypeBlock {
			b.WriteString("\n")
		}
		lines := func() {
			for i := 0; i < n.Lines().Len(); i++ {
				seg := n.Lines().At(i)
				b.Write(seg.Value(src))
			}
		}
		switch n := n.(type) {
		case *ast.Text:
			b.Write(n.Segment.Value(src))
			b.WriteString(" ")
		case *ast.String:
			b.Write(n.Value)
		case *ast.RawHTML:
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				b.Write(seg.Value(src))
			}
		case *ast.Link:
			b.Write(n.Destination)
		case *ast.Image:
			b.Write(n.Destination)
		case *ast.FencedCodeBlock:
			b.Write(n.Language(src))
			b.WriteString("\n")
			lines()
		case *ast.CodeBlock, *ast.HTMLBlock:
			lines()
		}
		return ast.WalkContinue, nil
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

// diffBodies marks what replacing oldBody with newBody changed, in newBody's
// block numbering: blocks with no match in the old body are new, blocks that
// rewrite an old one are modified, and old blocks left without a rewrite
// leave a deletion point. It returns nil when nothing changed at
// block granularity, or when the bodies are too large to diff.
func diffBodies(oldBody, newBody string) (marks []BlockMark) {
	// Cosmetic: a parser surprise must never fail the update.
	defer func() {
		if recover() != nil {
			marks = nil
		}
	}()
	keys := func(body string) []string {
		src := []byte(body)
		units := bodyUnits(docParser.Parse(text.NewReader(src)), src)
		out := make([]string, len(units))
		for i, u := range units {
			out[i] = u.key
		}
		return out
	}
	return diffKeys(keys(oldBody), keys(newBody))
}

// diffKeys matches the two block sequences by longest common subsequence,
// then aligns each stretch between matches by similarity (alignGap).
func diffKeys(old, cur []string) []BlockMark {
	n, m := len(old), len(cur)
	if n*m > maxDiffCells {
		return nil
	}
	// lcs[i][j] is the common subsequence length of old[i:] and cur[j:].
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if old[i] == cur[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var marks []BlockMark
	i, j, oi, cj := 0, 0, 0, 0 // oi, cj: start of the current gap
	for i < n && j < m {
		switch {
		case old[i] == cur[j]:
			marks = append(marks, alignGap(old[oi:i], cur[cj:j], cj)...)
			i, j = i+1, j+1
			oi, cj = i, j
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			j++
		}
	}
	marks = append(marks, alignGap(old[oi:], cur[cj:], cj)...)
	sortMarks(marks)
	return marks
}

// pairThreshold is the least word overlap (Jaccard) at which a block of the
// same kind counts as the rewrite of an old one instead of a new block.
const pairThreshold = 0.3

// similarity is how alike two block keys of one kind are, zero for different
// kinds: the larger of their word overlap and their shared start and end. A
// key is its kind, a space, then its text.
func similarity(a, b string) float64 {
	ka, ta, _ := strings.Cut(a, " ")
	kb, tb, _ := strings.Cut(b, " ")
	if ka != kb {
		return 0
	}
	words := map[string]int{}
	for _, w := range strings.Fields(ta) {
		words[w] |= 1
	}
	for _, w := range strings.Fields(tb) {
		words[w] |= 2
	}
	if len(words) == 0 {
		return 1
	}
	both := 0
	for _, in := range words {
		if in == 3 {
			both++
		}
	}
	// The shared start and end catch a one-word block whose word changed
	// a little, which shares no whole word.
	pre := 0
	for pre < min(len(ta), len(tb)) && ta[pre] == tb[pre] {
		pre++
	}
	suf := 0
	for suf < min(len(ta), len(tb))-pre && ta[len(ta)-1-suf] == tb[len(tb)-1-suf] {
		suf++
	}
	return max(float64(both)/float64(len(words)), float64(pre+suf)/float64(max(len(ta), len(tb))))
}

// alignGap marks one stretch between matched blocks: old blocks and new ones
// (curFrom is the first new block's number) are paired in order, maximizing
// total similarity; a pair is modified, an unpaired new block new, and old
// blocks left unpaired leave one deletion point before the next new block.
func alignGap(old, cur []string, curFrom int) []BlockMark {
	n, m := len(old), len(cur)
	if n == 0 && m == 0 {
		return nil
	}
	if n == m {
		// A stretch that swaps blocks one for one, kind for kind, is a
		// rewrite of each, however little the text has in common.
		same := true
		for i := range old {
			ki, _, _ := strings.Cut(old[i], " ")
			kj, _, _ := strings.Cut(cur[i], " ")
			same = same && ki == kj
		}
		if same {
			marks := make([]BlockMark, n)
			for i := range marks {
				marks[i] = BlockMark{curFrom + i, MarkMod}
			}
			return marks
		}
	}
	sim := func(i, j int) float64 {
		if s := similarity(old[i], cur[j]); s >= pairThreshold {
			return s
		}
		return 0
	}
	// best[i][j] is the best total similarity of old[i:] and cur[j:].
	best := make([][]float64, n+1)
	for i := range best {
		best[i] = make([]float64, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			best[i][j] = max(best[i+1][j], best[i][j+1])
			if s := sim(i, j); s > 0 {
				best[i][j] = max(best[i][j], best[i+1][j+1]+s)
			}
		}
	}
	var marks []BlockMark
	deleted := -1 // the block number of the last deletion point written
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && sim(i, j) > 0 && best[i][j] == best[i+1][j+1]+sim(i, j):
			marks = append(marks, BlockMark{curFrom + j, MarkMod})
			i, j = i+1, j+1
		case i < n && (j == m || best[i][j] == best[i+1][j]):
			if deleted != curFrom+j {
				marks = append(marks, BlockMark{curFrom + j, MarkDel})
				deleted = curFrom + j
			}
			i++
		default:
			marks = append(marks, BlockMark{curFrom + j, MarkNew})
			j++
		}
	}
	return marks
}

// sortMarks orders marks by block, a deletion point before the block's own
// mark.
func sortMarks(marks []BlockMark) {
	sort.SliceStable(marks, func(i, j int) bool {
		a, b := marks[i], marks[j]
		if a.Block != b.Block {
			return a.Block < b.Block
		}
		return a.Kind == MarkDel && b.Kind != MarkDel
	})
}

var kindChange = ast.NewNodeKind("WhiteboardChange")

// changeNode is a change mark in a parsed body; it renders as an HTML comment
// the page script reads (assets/chg.js).
type changeNode struct {
	ast.BaseBlock
	kind string
}

func (n *changeNode) Kind() ast.NodeKind { return kindChange }

func (n *changeNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Mark": n.kind}, nil)
}

// afterLast is the kind of the mark that sits after the body's last block (a
// deletion at the end); the body's own marks never use it.
const afterLast = "delafter"

// applyMarks inserts marks into a parsed body. A mark outside the body's
// blocks (a stale mark after a parser difference) is dropped.
func applyMarks(doc ast.Node, src []byte, marks []BlockMark) {
	units := bodyUnits(doc, src)
	for _, mk := range marks {
		switch {
		case mk.Block >= 0 && mk.Block < len(units):
			node := units[mk.Block].node
			parent := node.Parent()
			parent.InsertBefore(parent, node, &changeNode{kind: mk.Kind})
		case mk.Kind == MarkDel && mk.Block == len(units) && len(units) > 0:
			node := units[len(units)-1].node
			parent := node.Parent()
			parent.InsertAfter(parent, node, &changeNode{kind: afterLast})
		}
	}
}

// renderChange writes a change mark as an HTML comment: a comment is no
// element, so it leaves :first-child and :last-child rules and the layout
// alone.
func renderChange(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<!--wbchg:" + n.(*changeNode).kind + "-->\n")
	}
	return ast.WalkSkipChildren, nil
}
