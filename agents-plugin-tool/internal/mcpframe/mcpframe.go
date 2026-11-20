// Package mcpframe defines the shared MCP request size bound and bounded
// framing helpers used by both the HTTP proxy and the stdio server.
package mcpframe

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strconv"
)

// MaxRequestBytes is the maximum payload size (in bytes) of a single MCP
// JSON-RPC request across both HTTP (/mcp/session) and stdio transports.
//
// It sits above normal tool payloads (including large whiteboard.update SVGs
// and complete path.write review artifacts) while keeping per-request memory
// allocation finite.
const MaxRequestBytes = 64 << 20

// maxExtractedIDBytes caps a recovered top-level JSON-RPC "id" echoed back in
// an oversize-request error response so an adversarial huge id cannot force a
// multi-megabyte error body.
const maxExtractedIDBytes = 4096

// TooLargeMessage returns the shared JSON-RPC -32600 error text for requests
// that exceed limit bytes.
func TooLargeMessage(limit int) string {
	return "message exceeds " + strconv.Itoa(limit) + " bytes"
}

// ReadLine reads one newline-delimited request line from r while bounding
// allocation to maxBytes payload bytes (excluding '\n' and an optional
// preceding '\r').
//
// When a line exceeds maxBytes, ReadLine retains at most a maxBytes prefix,
// discards the remainder of that line through '\n' (or EOF) without further
// allocation, and returns (prefix, true, nil) so the caller can reply with a
// per-request JSON-RPC error and keep serving subsequent lines.
//
// io.EOF is returned only when no bytes remain before EOF.
func ReadLine(r *bufio.Reader, maxBytes int) (line []byte, tooLarge bool, err error) {
	if maxBytes < 0 {
		maxBytes = 0
	}
	frag, err := r.ReadSlice('\n')
	if err == nil {
		trimmed := trimLineEnding(frag)
		if len(trimmed) > maxBytes {
			return bytes.Clone(trimmed[:maxBytes]), true, nil
		}
		if len(trimmed) == 0 {
			return nil, false, nil
		}
		return bytes.Clone(trimmed), false, nil
	}
	if err == io.EOF {
		if len(frag) == 0 {
			return nil, false, io.EOF
		}
		if len(frag) > maxBytes {
			return bytes.Clone(frag[:maxBytes]), true, nil
		}
		return bytes.Clone(frag), false, nil
	}
	if err != bufio.ErrBufferFull {
		return nil, false, err
	}

	// Multi-chunk line: keep at most maxBytes+1 bytes before '\n' so a trailing
	// '\r' in a CRLF line at exact maxBytes does not trigger a false oversize.
	keepLimit := maxBytes + 1
	var buf []byte
	over := false
	consume := func(chunk []byte) {
		if over {
			return
		}
		if len(buf)+len(chunk) <= keepLimit {
			buf = appendCapped(buf, chunk, keepLimit)
			return
		}
		if len(buf) < maxBytes {
			need := maxBytes - len(buf)
			if need > len(chunk) {
				need = len(chunk)
			}
			buf = appendCapped(buf, chunk[:need], maxBytes)
		} else if len(buf) > maxBytes {
			buf = buf[:maxBytes]
		}
		over = true
	}

	consume(frag)
	for {
		frag, err = r.ReadSlice('\n')
		switch err {
		case bufio.ErrBufferFull:
			consume(frag)
		case nil:
			if len(frag) > 0 && frag[len(frag)-1] == '\n' {
				frag = frag[:len(frag)-1]
			}
			consume(frag)
			if over {
				return trimPrefix(buf, maxBytes), true, nil
			}
			if len(buf) > 0 && buf[len(buf)-1] == '\r' {
				buf = buf[:len(buf)-1]
			}
			if len(buf) > maxBytes {
				return buf[:maxBytes], true, nil
			}
			return buf, false, nil
		case io.EOF:
			consume(frag)
			if over || len(buf) > maxBytes {
				return trimPrefix(buf, maxBytes), true, nil
			}
			return buf, false, nil
		default:
			return nil, false, err
		}
	}
}

func trimLineEnding(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
		if len(b) > 0 && b[len(b)-1] == '\r' {
			b = b[:len(b)-1]
		}
	}
	return b
}

func trimPrefix(buf []byte, maxBytes int) []byte {
	if len(buf) > maxBytes {
		return buf[:maxBytes]
	}
	return buf
}

func appendCapped(buf, chunk []byte, limit int) []byte {
	if len(chunk) == 0 {
		return buf
	}
	need := len(buf) + len(chunk)
	if need <= cap(buf) {
		return append(buf, chunk...)
	}
	newCap := cap(buf) * 2
	if newCap < 4096 {
		newCap = 4096
	}
	if newCap < need {
		newCap = need
	}
	if limit > 0 && newCap > limit {
		newCap = limit
	}
	if newCap < need {
		newCap = need
	}
	next := make([]byte, len(buf), newCap)
	copy(next, buf)
	return append(next, chunk...)
}

// ExtractID recovers a complete top-level JSON-RPC "id" (string, number, or
// null) from a possibly truncated JSON object prefix without allocating to
// skip sibling values. Returns nil when prefix does not start a JSON object or
// no complete valid top-level "id" is parseable within prefix.
func ExtractID(prefix []byte) json.RawMessage {
	i := skipWS(prefix, 0)
	if i >= len(prefix) || prefix[i] != '{' {
		return nil
	}
	i = skipWS(prefix, i+1)
	for i < len(prefix) {
		if prefix[i] == '}' {
			return nil
		}
		if prefix[i] != '"' {
			return nil
		}
		keyEnd, ok := scanJSONString(prefix, i)
		if !ok {
			return nil
		}
		keyRaw := prefix[i:keyEnd]
		i = skipWS(prefix, keyEnd)
		if i >= len(prefix) || prefix[i] != ':' {
			return nil
		}
		i = skipWS(prefix, i+1)
		if i >= len(prefix) {
			return nil
		}
		if isIDKey(keyRaw) {
			return parseIDValue(prefix, i)
		}
		next, ok := skipJSONValue(prefix, i)
		if !ok {
			return nil
		}
		i = skipWS(prefix, next)
		if i >= len(prefix) {
			return nil
		}
		switch prefix[i] {
		case ',':
			i = skipWS(prefix, i+1)
		case '}':
			return nil
		default:
			return nil
		}
	}
	return nil
}

func isIDKey(raw []byte) bool {
	if bytes.Equal(raw, []byte(`"id"`)) {
		return true
	}
	if len(raw) <= 64 && bytes.IndexByte(raw, '\\') >= 0 {
		var key string
		return json.Unmarshal(raw, &key) == nil && key == "id"
	}
	return false
}

func parseIDValue(prefix []byte, start int) json.RawMessage {
	switch prefix[start] {
	case '"':
		end, ok := scanJSONString(prefix, start)
		if !ok || end-start > maxExtractedIDBytes {
			return nil
		}
		val := prefix[start:end]
		if !json.Valid(val) {
			return nil
		}
		return bytes.Clone(val)
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		end := start
		for end < len(prefix) && !isJSONDelimiter(prefix[end]) {
			end++
		}
		// A number touching the end of prefix may be truncated mid-digits.
		if end >= len(prefix) || end-start > maxExtractedIDBytes {
			return nil
		}
		val := prefix[start:end]
		if !json.Valid(val) {
			return nil
		}
		return bytes.Clone(val)
	case 'n':
		if bytes.HasPrefix(prefix[start:], []byte("null")) {
			end := start + 4
			if end < len(prefix) && isJSONDelimiter(prefix[end]) {
				return json.RawMessage("null")
			}
		}
		return nil
	default:
		return nil
	}
}

func skipJSONValue(prefix []byte, start int) (int, bool) {
	switch prefix[start] {
	case '"':
		return scanJSONString(prefix, start)
	case '{', '[':
		depth := 1
		i := start + 1
		for i < len(prefix) {
			switch prefix[i] {
			case '"':
				next, ok := scanJSONString(prefix, i)
				if !ok {
					return 0, false
				}
				i = next
			case '{', '[':
				depth++
				i++
			case '}', ']':
				depth--
				i++
				if depth == 0 {
					return i, true
				}
			default:
				i++
			}
		}
		return 0, false
	default:
		i := start
		for i < len(prefix) && !isJSONDelimiter(prefix[i]) {
			i++
		}
		if i == start || i >= len(prefix) {
			return 0, false
		}
		return i, true
	}
}

func scanJSONString(b []byte, start int) (int, bool) {
	i := start + 1
	for i < len(b) {
		switch b[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1, true
		default:
			i++
		}
	}
	return 0, false
}

func skipWS(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}
	return i
}

func isJSONDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', ',', '}', ']':
		return true
	default:
		return false
	}
}
