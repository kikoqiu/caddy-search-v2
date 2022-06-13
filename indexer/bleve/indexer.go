package bleve

import (
	"fmt"
	"time"

	bleve "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/highlight/highlighter/html"
	"github.com/caddyserver/caddy/v2/modules/caddy-search/indexer"
)

type bleveIndexer struct {
	bleve bleve.Index
}

// Bleve's record data struct
type indexRecord struct {
	Path     string
	Title    string
	Body     string
	Modified time.Time
	Indexed  time.Time
}

// Record method get existent or creates a new Record to be saved/updated in the indexer
func (i *bleveIndexer) Record(path string) indexer.Record {
	record := &Record{}
	record.path = path
	record.fullPath = ""
	record.title = ""
	record.document = make(map[string]interface{})
	record.ignored = false
	record.loaded = false
	record.body = make([]byte, 0)
	record.indexed = time.Time{}
	record.modified = time.Time{}
	record.indexer = i
	record.mimetype = ""
	return record
}

// Search method lookup for records using a query
func (i *bleveIndexer) Search(q string) (records []indexer.Record) {
	query := bleve.NewQueryStringQuery(q)
	request := bleve.NewSearchRequest(query)
	request.Highlight = bleve.NewHighlightWithStyle(html.Name) //bleve.NewHighlight()
	result, err := i.bleve.Search(request)
	if err != nil { // an empty query would cause this
		return
	}

	for _, match := range result.Hits {
		rec := i.Record(match.ID)
		loaded := rec.Load()

		if !loaded {
			continue
		}

		if len(match.Fragments["Body"]) > 0 {
			rec.SetBody([]byte(match.Fragments["Body"][0]))
		}

		records = append(records, rec)
	}

	return
}

// Index sends the new record to the pipeline
func (i *bleveIndexer) Index(in indexer.Record) {
	rec, ok := in.(*Record)
	if !ok {
		return
	}
	i.index(rec)
}

// index is the step that indexes the document
func (i *bleveIndexer) index(rec *Record) {
	if rec != nil && len(rec.body) > 0 && !rec.Ignored() {
		rec.SetIndexed(time.Now())
		fmt.Println(rec.FullPath())

		r := indexRecord{
			Path:     rec.Path(),
			Title:    rec.Title(),
			Body:     string(rec.body),
			Modified: rec.Modified(),
			Indexed:  rec.Indexed(),
		}

		//t := time.Now()
		i.bleve.Index(rec.Path(), r)
		//fmt.Printf("1: %v\n", time.Since(t))
	}
}

// New creates a new instance for this indexer
func New(name string, analyzer string) (*bleveIndexer, error) {
	blv, err := openIndex(name, analyzer)
	if err != nil {
		return nil, err
	}

	indxr := &bleveIndexer{}
	indxr.bleve = blv

	return indxr, nil
}
func openIndex(name string, analyzer string) (bleve.Index, error) {
	textFieldMapping := bleve.NewTextFieldMapping()

	doc := bleve.NewDocumentMapping()
	doc.AddFieldMappingsAt("Path", textFieldMapping)
	doc.AddFieldMappingsAt("Title", textFieldMapping)
	doc.AddFieldMappingsAt("Body", textFieldMapping)
	doc.AddFieldMappingsAt("Modified", bleve.NewDateTimeFieldMapping())
	doc.AddFieldMappingsAt("Indexed", bleve.NewDateTimeFieldMapping())

	indexMap := bleve.NewIndexMapping()
	switch analyzer {
	case "sego":
		AddSegoChineseAnalyzer(indexMap)
	}
	indexMap.DefaultAnalyzer = analyzer
	indexMap.AddDocumentMapping("document", doc)

	//blv, err := bleve.New(name, indexMap)
	blv, err := bleve.NewUsing(name, indexMap, "scorch", "scorch", nil)

	if err != nil {
		blv, err = bleve.Open(name)
		if err != nil {
			return nil, err
		}
	}

	return blv, nil
}
