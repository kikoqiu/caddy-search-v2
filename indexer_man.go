package search

import (
	"bytes"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/caddyserver/caddy/v2/modules/caddy-search/indexer"
	"github.com/gabriel-vasile/mimetype"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

var bm = bluemonday.StrictPolicy() //bluemonday.UGCPolicy()

// NewIndexerManager creates a new Pipeline instance
func NewIndexerManager(config *Search, MaxFileSize int, indxr indexer.Handler) (*IndexerManager, error) {
	ppl := &IndexerManager{
		config:      config,
		indexer:     indxr,
		MaxFileSize: MaxFileSize,
	}

	ppl.queue = make(chan indexer.Record, config.NumWorkers)
	for i := 0; i < config.NumWorkers; i++ {
		go func() {
			for {
				{
					rc := <-ppl.queue
					if rc.Ignored() {
						continue
					}

					if !ppl.ValidatePath(rc.Path()) {
						rc.Ignore()
						continue
					}

					indoc := indxr.Record(rc.Path())
					if indoc.Load() {
						if indoc.Indexed().After(rc.Modified()) {
							log.Printf("Ignored: %v \n Indexed %v > Modified %v ", rc.Path(), indoc.Indexed().Format("2006-01-02 15:04:05"), rc.Modified().Format("2006-01-02 15:04:05"))
							rc.Ignore()
							continue
						}
					}

					if len(rc.Body()) <= 0 {
						var detectedMIME *mimetype.MIME = nil

						if rc.MimeType() != "" {
							detectedMIME = mimetype.Lookup(strings.Split(rc.MimeType(), ";")[0])
						}
						if detectedMIME == nil {
							detectedMIME, _ = mimetype.DetectFile(rc.FullPath())
						}
						if detectedMIME != nil {
							rc.SetMimeType(detectedMIME.String())
						}

						isBinary := true
						for mtype := detectedMIME; mtype != nil; mtype = mtype.Parent() {
							if mtype.Is("text/plain") {
								isBinary = false
								break
							}
						}
						if isBinary {
							rc.Ignore()
							continue
						}

						in, err := os.Open(rc.FullPath())
						if err != nil {
							rc.Ignore()
							continue
						}
						io.Copy(rc, in)
						in.Close()
					}

					ppl.index(rc)
				}
				runtime.GC()
			}
		}()
	}

	return ppl, nil
}

// IndexerManager is the structure that holds search's pipeline infos and methods
type IndexerManager struct {
	config      *Search
	indexer     indexer.Handler
	queue       chan indexer.Record
	MaxFileSize int
}

// Feed is the step of the pipeline that feeds valid documents to the indexer.
func (p *IndexerManager) Feed(record indexer.Record) {
	p.queue <- record
}

func getHtmlTitle(r io.Reader, defval string) (result string, err error) {
	z := html.NewTokenizer(r)
	result = ""
	intag := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			err = z.Err()
			return defval, err
		case html.StartTagToken:
			tn, _ := z.TagName()
			if strings.ToLower(string(tn)) == "title" {
				intag = true
			}
		case html.EndTagToken:
			//tn, _ := z.TagName()
			if /*strings.ToLower(string(tn)) == "title" &&*/ intag {
				return result, nil
			}
		case html.TextToken:
			if intag {
				result = result + string(z.Text())
			}

		}
	}
}

// index is the step of the pipeline that pipes valid documents to the indexer.
func (p *IndexerManager) index(record indexer.Record) {
	if record.Ignored() {
		return
	}

	if len(record.Body()) > p.MaxFileSize {
		record.Ignore()
		return
	}

	var detectedMIME *mimetype.MIME = mimetype.Lookup(strings.Split(record.MimeType(), ";")[0])

	if detectedMIME != nil && detectedMIME.Is("text/html") {
		body := bytes.NewReader(record.Body())
		title, _ := getHtmlTitle(body, path.Base(record.Path()))
		record.SetTitle(title)
		stripped := bm.SanitizeBytes(record.Body())
		log.Printf("Size %v/%v: %v", len(stripped), len(record.Body()), record.FullPath())
		record.SetBody(stripped)
	} else {
		log.Printf("Size %v: %v", len(record.Body()), record.FullPath())
		record.SetTitle(path.Base(record.Path()))
	}

	if !record.Ignored() {
		p.indexer.Index(record)
	}
}

// ValidatePath is the method that checks if the target page can be indexed
func (p *IndexerManager) ValidatePath(path string) bool {
	for _, pa := range p.config.ExcludePaths {
		if pa.MatchString(path) {
			return false
		}
	}

	for _, pa := range p.config.IncludePaths {
		if pa.MatchString(path) {
			return true
		}
	}

	return false
}
