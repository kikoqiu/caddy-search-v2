package bleve

import (
	"bufio"
	"embed"
	"io"

	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/registry"
	"github.com/caddyserver/caddy/v2/modules/caddy-search/indexer/bleve/sego"
)

//go:embed sego/dicts/*
var dicts embed.FS

const defaultDict = "sego/dicts/default.txt"

// SegoTokenizer is the beleve tokenizer for jiebago.
type SegoTokenizer struct {
	seg        sego.Segmenter
	searchMode bool
}

// Tokenize cuts input into bleve token stream.
func (seg *SegoTokenizer) Tokenize(input []byte) analysis.TokenStream {
	segments := seg.seg.Segment(input)
	ret := sego.SegmentsToTokenStream(input, segments, seg.searchMode)
	return ret
}

func SegoTokenizerConstructor(config map[string]interface{}, cache *registry.Cache) (analysis.Tokenizer, error) {
	var seg sego.Segmenter

	dictFilePath, ok := config["dict"].(string)
	if ok {
		err := seg.LoadDictionary(dictFilePath)
		if err != nil {
			return nil, err
		}
	} else {
		file, err := dicts.Open(defaultDict)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		err = seg.LoadDictionaryFromReaders([]io.Reader{bufio.NewReader(file)})
		if err != nil {
			return nil, err
		}
	}

	searchMode, ok := config["search"].(bool)
	if !ok {
		searchMode = true
	}

	return &SegoTokenizer{
		seg:        seg,
		searchMode: searchMode,
	}, nil
}

func AddSegoChineseAnalyzer(indexMapping *mapping.IndexMappingImpl) error {
	err := indexMapping.AddCustomTokenizer("sego",
		map[string]interface{}{
			//"dict": "",
			"search": true,
			"type":   "sego",
		})
	if err != nil {
		return err
	}

	err = indexMapping.AddCustomAnalyzer("sego",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": "sego",
			"token_filters": []string{
				"possessive_en",
				"to_lower",
				"stop_en",
			},
		})
	return err
}

func init() {
	registry.RegisterTokenizer("sego", SegoTokenizerConstructor)
}
