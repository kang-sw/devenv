package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// A panel tree is a JSON value laid out as foldable nodes: an object's
// fields and an array's elements are children, a scalar is a leaf. The
// agenda panel builds one per blob.

// Kinds of TreeNode.
const (
	TreeObject = "obj"
	TreeArray  = "arr"
	TreeString = "str"
	TreeNumber = "num"
	TreeBool   = "bool"
	TreeNull   = "null"
)

// treeLongRunes is the longest string shown whole on its row; a longer one,
// or one with a line break, folds into a one-line preview that opens to the
// whole value.
const treeLongRunes = 60

// treeSummaryRunes bounds a folded object's field-name preview.
const treeSummaryRunes = 48

// TreeNode is one node of a panel tree.
type TreeNode struct {
	// Key is the node's field name, its array index, or the root's name.
	Key string
	// Path identifies the node across renders, so a page keeps what the
	// reader opened or closed when the panel is redrawn: the root's name,
	// then each field name or index, JSON Pointer escaped, joined by "/".
	Path string
	Kind string
	// Value is a scalar's text, or a container's folded preview: an object's
	// field names, an array's length. An empty container shows "{}" or "[]"
	// and has no children.
	Value string
	// Long marks a scalar too long for its row (see treeLongRunes).
	Long bool
	// Open is a container's initial state.
	Open     bool
	Children []TreeNode
}

// IsContainer reports whether the node folds over children.
func (n TreeNode) IsContainer() bool { return len(n.Children) > 0 }

// JSONTree lays raw out as a tree named key, whose path starts with
// pathPrefix. The root is open and every deeper container folded. Object
// fields keep their stored order. A value that is not valid JSON becomes one
// string leaf of its text.
func JSONTree(pathPrefix, key string, raw json.RawMessage) TreeNode {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	path := pathPrefix + escapeTreeKey(key)
	node, err := decodeTreeNode(dec, key, path, 0)
	if err != nil {
		return treeScalar(key, path, TreeString, strings.TrimSpace(string(raw)))
	}
	return node
}

func decodeTreeNode(dec *json.Decoder, key, path string, depth int) (TreeNode, error) {
	tok, err := dec.Token()
	if err != nil {
		return TreeNode{}, err
	}
	switch v := tok.(type) {
	case json.Delim:
		node := TreeNode{Key: key, Path: path, Open: depth == 0}
		var names []string
		switch v {
		case '{':
			node.Kind = TreeObject
			for dec.More() {
				nameTok, err := dec.Token()
				if err != nil {
					return TreeNode{}, err
				}
				name, _ := nameTok.(string)
				child, err := decodeTreeNode(dec, name, path+"/"+escapeTreeKey(name), depth+1)
				if err != nil {
					return TreeNode{}, err
				}
				node.Children = append(node.Children, child)
				names = append(names, name)
			}
		case '[':
			node.Kind = TreeArray
			for i := 0; dec.More(); i++ {
				index := fmt.Sprint(i)
				child, err := decodeTreeNode(dec, index, path+"/"+index, depth+1)
				if err != nil {
					return TreeNode{}, err
				}
				node.Children = append(node.Children, child)
			}
		default:
			return TreeNode{}, fmt.Errorf("unexpected %v", v)
		}
		if _, err := dec.Token(); err != nil { // the closing delimiter
			return TreeNode{}, err
		}
		node.Value = containerPreview(node.Kind, names, len(node.Children))
		return node, nil
	case string:
		return treeScalar(key, path, TreeString, v), nil
	case json.Number:
		return treeScalar(key, path, TreeNumber, v.String()), nil
	case bool:
		return treeScalar(key, path, TreeBool, fmt.Sprint(v)), nil
	default:
		return treeScalar(key, path, TreeNull, "null"), nil
	}
}

func treeScalar(key, path, kind, value string) TreeNode {
	long := kind == TreeString && (utf8.RuneCountInString(value) > treeLongRunes || strings.ContainsAny(value, "\r\n"))
	return TreeNode{Key: key, Path: path, Kind: kind, Value: value, Long: long}
}

// containerPreview is a container's folded text: "{a, b, …}" for an object,
// "[n]" for an array, "{}" or "[]" when empty.
func containerPreview(kind string, names []string, n int) string {
	if kind == TreeArray {
		if n == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d]", n)
	}
	if n == 0 {
		return "{}"
	}
	inner := strings.Join(names, ", ")
	if utf8.RuneCountInString(inner) > treeSummaryRunes {
		inner = string([]rune(inner)[:treeSummaryRunes-1]) + "…"
	}
	return "{" + inner + "}"
}

// escapeTreeKey escapes a path segment the way JSON Pointer does, so a field
// name holding "/" cannot alias another path.
func escapeTreeKey(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}
