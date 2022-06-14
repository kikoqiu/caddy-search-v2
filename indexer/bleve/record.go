package bleve

import (
	"time"

	index "github.com/blevesearch/bleve_index_api"
)

// Record handles indexer's data
type Record struct {
	indexer  *bleveIndexer
	path     string
	fullPath string
	title    string
	document map[string]interface{}
	body     []byte
	loaded   bool
	modified time.Time
	ignored  bool
	indexed  time.Time
	mimetype string
}

// Path returns Record's path
func (r *Record) Path() string {
	return r.path
}

// FullPath returns Record's fullpath
func (r *Record) FullPath() string {
	return r.fullPath
}

// SetFullPath defines a new fullpath for the record
func (r *Record) SetFullPath(fp string) {
	r.fullPath = fp
}

// Title returns Record's title
func (r *Record) Title() string {
	return r.title
}

// SetTitle replaces Record's title
func (r *Record) SetTitle(title string) {
	r.title = title
}

// Modified returns Record's Modified
func (r *Record) Modified() time.Time {
	return r.modified
}

// SetModified defines new modification time for this record
func (r *Record) SetModified(mod time.Time) {
	r.modified = mod
}

// SetBody replaces the actual body
func (r *Record) SetBody(body []byte) {
	r.body = body
}

// Body returns Record's body
func (r *Record) Body() []byte {
	return r.body
}
func GetDateTime(f index.Field) time.Time {
	if f != nil {
		ret, err := f.(index.DateTimeField).DateTime()
		if err == nil {
			return ret.Local()
		}
	}
	return time.Unix(0, 0)
}

// Load this record from the indexer.
func (r *Record) Load() bool {
	doc, err := r.indexer.bleve.Document(r.path)
	if err != nil || doc == nil {
		r.loaded = true
		return false
	}

	result := make(map[string]index.Field)

	doc.VisitFields(func(field index.Field) {
		name := field.Name()
		value := field
		result[name] = value
	})

	r.modified = GetDateTime(result["Modified"])
	r.indexed = GetDateTime(result["Indexed"])

	r.SetBody(result["Body"].Value())
	r.title = string(result["Title"].Value())

	r.loaded = true

	return true
}

// Write is the writing method for a Record
func (r *Record) Write(p []byte) (int, error) {
	r.body = append(r.body, p...)
	return len(p), nil
}

// Ignore flag this record as ignored
func (r *Record) Ignore() {
	r.ignored = true
}

// Ignored returns if this record is ignored
func (r *Record) Ignored() bool {
	return r.ignored
}

// Indexed returns the indexing time (if indexed)
func (r *Record) Indexed() time.Time {
	return r.indexed
}

// SetIndexed define the time that this record has been indexed
func (r *Record) SetIndexed(index time.Time) {
	r.indexed = index
}

func (r *Record) MimeType() string {
	return r.mimetype
}

func (r *Record) SetMimeType(val string) {
	r.mimetype = val
}
