package textreader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	DefaultTailLines  = 40
	MaxTailLines      = 1000
	DefaultReadLimit  = 4096
	MaxReadLimit      = 65536
	DefaultMaxMatches = 50
	MaxMatchesLimit   = 1000
)

type ReadResult struct {
	Text       string `json:"text"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	Limit      int64  `json:"limit"`
	Size       int64  `json:"size"`
	EOF        bool   `json:"eof"`
}

type GrepMatch struct {
	Path   string   `json:"path,omitempty"`
	Line   int      `json:"line"`
	Text   string   `json:"text"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

type GrepResult struct {
	Matches   []GrepMatch `json:"matches"`
	Truncated bool        `json:"truncated"`
}

func Tail(path string, lines int) (string, error) {
	if lines <= 0 {
		lines = DefaultTailLines
	}
	if lines > MaxTailLines {
		lines = MaxTailLines
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ring := make([]string, lines)
	count := 0
	for scanner.Scan() {
		ring[count%lines] = scanner.Text()
		count++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if count == 0 {
		return "", nil
	}
	n := count
	if n > lines {
		n = lines
	}
	out := make([]string, 0, n)
	start := count - n
	for i := 0; i < n; i++ {
		out = append(out, ring[(start+i)%lines])
	}
	return strings.Join(out, "\n") + "\n", nil
}

func Read(path string, offset, limit int64) (ReadResult, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = DefaultReadLimit
	}
	if limit > MaxReadLimit {
		limit = MaxReadLimit
	}
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return ReadResult{Offset: offset, NextOffset: offset, Limit: limit, EOF: true}, nil
	}
	if err != nil {
		return ReadResult{}, err
	}
	if offset > st.Size() {
		offset = st.Size()
	}
	f, err := os.Open(path)
	if err != nil {
		return ReadResult{}, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ReadResult{}, err
	}
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return ReadResult{}, err
	}
	next := offset + int64(n)
	return ReadResult{Text: string(buf[:n]), Offset: offset, NextOffset: next, Limit: limit, Size: st.Size(), EOF: next >= st.Size()}, nil
}

func Grep(paths []string, pattern string, before, after, maxMatches int, regex bool) (GrepResult, error) {
	if pattern == "" {
		return GrepResult{}, fmt.Errorf("pattern is required")
	}
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	if maxMatches <= 0 {
		maxMatches = DefaultMaxMatches
	}
	if maxMatches > MaxMatchesLimit {
		maxMatches = MaxMatchesLimit
	}
	var re *regexp.Regexp
	var err error
	if regex {
		re, err = regexp.Compile(pattern)
		if err != nil {
			return GrepResult{}, err
		}
	}
	out := GrepResult{}
	for _, path := range paths {
		lines, err := readLines(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return out, err
		}
		for i, line := range lines {
			matched := strings.Contains(line, pattern)
			if regex {
				matched = re.MatchString(line)
			}
			if !matched {
				continue
			}
			start := i - before
			if start < 0 {
				start = 0
			}
			end := i + 1 + after
			if end > len(lines) {
				end = len(lines)
			}
			m := GrepMatch{Path: path, Line: i + 1, Text: line}
			if start < i {
				m.Before = append([]string(nil), lines[start:i]...)
			}
			if i+1 < end {
				m.After = append([]string(nil), lines[i+1:end]...)
			}
			out.Matches = append(out.Matches, m)
			if len(out.Matches) >= maxMatches {
				out.Truncated = true
				return out, nil
			}
		}
	}
	return out, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil && err != bufio.ErrFinalToken {
		return nil, err
	}
	return lines, nil
}

func Join(paths []string) (string, error) {
	var b bytes.Buffer
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		b.Write(raw)
	}
	return b.String(), nil
}
