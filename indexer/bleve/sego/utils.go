package sego

import (
	"bytes"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/blevesearch/bleve/v2/analysis"
)

// 输出分词结果为字符串
//
// 有两种输出模式，以"中华人民共和国"为例
//
//  普通模式（searchMode=false）输出一个分词"中华人民共和国/ns "
//  搜索模式（searchMode=true） 输出普通模式的再细致切分：
//      "中华/nz 人民/n 共和/nz 共和国/ns 人民共和国/nt 中华人民共和国/ns "
//
// 搜索模式主要用于给搜索引擎提供尽可能多的关键字，详情请见Token结构体的注释。
func SegmentsToString(segs []Segment, searchMode bool) (output string) {
	if searchMode {
		for _, seg := range segs {
			output += tokenToString(seg.token)
		}
	} else {
		for _, seg := range segs {
			output += fmt.Sprintf(
				"%s/%s ", textSliceToString(seg.token.text), seg.token.pos)
		}
	}
	return
}

func tokenToString(token *Token) (output string) {
	hasOnlyTerminalToken := true
	for _, s := range token.segments {
		if len(s.token.segments) > 1 {
			hasOnlyTerminalToken = false
		}
	}

	if !hasOnlyTerminalToken {
		for _, s := range token.segments {
			if s != nil {
				output += tokenToString(s.token)
			}
		}
	}
	output += fmt.Sprintf("%s/%s ", textSliceToString(token.text), token.pos)
	return
}

//var ideographRegexp = regexp.MustCompile(`\p{Han}+`)

func detectTokenType(term []byte) analysis.TokenType {
	r, size := utf8.DecodeRune(term)
	if size <= 2 {
		if unicode.IsLetter(r) {
			return analysis.AlphaNumeric
		} else if unicode.IsNumber(r) {
			return analysis.Numeric
		} else {
			return analysis.AlphaNumeric
		}
	}
	return analysis.Ideographic
	/*if ideographRegexp.MatchString(term) {
		return analysis.Ideographic
	}
	_, err := strconv.ParseFloat(term, 64)
	if err == nil {
		return analysis.Numeric
	}
	return analysis.AlphaNumeric*/
}

func SegmentsToTokenStream(field []byte, segs []Segment, searchMode bool) analysis.TokenStream {
	output := make(analysis.TokenStream, 0)
	if searchMode {
		for _, seg := range segs {
			output = segToTokenStream(output, field, &seg, 0)
		}
		for pos, token := range output {
			token.Position = pos + 1
		}
	} else {
		for pos, seg := range segs {
			//text := seg.token.Text()
			token := analysis.Token{
				Term:     field[seg.start:seg.end],
				Start:    seg.start,
				End:      seg.end,
				Position: pos + 1,
				Type:     detectTokenType(field[seg.start:seg.end]),
			}
			output = append(output, &token)
		}
	}
	return output
}

func segToTokenStream(output analysis.TokenStream, field []byte, seg *Segment, offset int) analysis.TokenStream {
	hasOnlyTerminalToken := true
	for _, s := range seg.token.segments {
		if len(s.token.segments) > 1 {
			hasOnlyTerminalToken = false
		}
	}

	if !hasOnlyTerminalToken {
		for _, s := range seg.token.segments {
			output = segToTokenStream(output, field, s, offset+seg.start)
		}
	}

	//text := seg.token.Text()
	token := analysis.Token{
		//Term:  []byte(text),
		Term:  field[offset+seg.start : offset+seg.end],
		Start: offset + seg.start,
		End:   offset + seg.end,
		Type:  detectTokenType(field[offset+seg.start : offset+seg.end]),
	}

	if len(token.Term) == 1 && unicode.IsSpace(rune(token.Term[0])) {
		return output
	}

	/*if len(output) > 0 {
		last := output[len(output)-1]
		if len(last.Term) == 1 && unicode.IsSpace(rune(last.Term[0])) {
			if len(token.Term) == 1 && unicode.IsSpace(rune(token.Term[0])) {
				return output
			}
		}
	}*/

	output = append(output, &token)
	return output
}

// 输出分词结果到一个字符串slice
//
// 有两种输出模式，以"中华人民共和国"为例
//
//  普通模式（searchMode=false）输出一个分词"[中华人民共和国]"
//  搜索模式（searchMode=true） 输出普通模式的再细致切分：
//      "[中华 人民 共和 共和国 人民共和国 中华人民共和国]"
//
// 搜索模式主要用于给搜索引擎提供尽可能多的关键字，详情请见Token结构体的注释。

func SegmentsToSlice(segs []Segment, searchMode bool) (output []string) {
	if searchMode {
		for _, seg := range segs {
			output = append(output, tokenToSlice(seg.token)...)
		}
	} else {
		for _, seg := range segs {
			output = append(output, seg.token.Text())
		}
	}
	return
}

func tokenToSlice(token *Token) (output []string) {
	hasOnlyTerminalToken := true
	for _, s := range token.segments {
		if len(s.token.segments) > 1 {
			hasOnlyTerminalToken = false
		}
	}
	if !hasOnlyTerminalToken {
		for _, s := range token.segments {
			output = append(output, tokenToSlice(s.token)...)
		}
	}
	output = append(output, textSliceToString(token.text))
	return output
}

// 将多个字元拼接一个字符串输出
func textSliceToString(text []Text) string {
	return Join(text)
}

func Join(a []Text) string {
	switch len(a) {
	case 0:
		return ""
	case 1:
		return string(a[0])
	case 2:
		// Special case for common small values.
		// Remove if golang.org/issue/6714 is fixed
		return string(a[0]) + string(a[1])
	case 3:
		// Special case for common small values.
		// Remove if golang.org/issue/6714 is fixed
		return string(a[0]) + string(a[1]) + string(a[2])
	}
	n := 0
	for i := 0; i < len(a); i++ {
		n += len(a[i])
	}

	b := make([]byte, n)
	bp := copy(b, a[0])
	for _, s := range a[1:] {
		bp += copy(b[bp:], s)
	}
	return string(b)
}

// 返回多个字元的字节总长度
func textSliceByteLength(text []Text) (length int) {
	for _, word := range text {
		length += len(word)
	}
	return
}

func textSliceToBytes(text []Text) []byte {
	var buf bytes.Buffer
	for _, word := range text {
		buf.Write(word)
	}
	return buf.Bytes()
}
