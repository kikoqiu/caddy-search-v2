package search

import (
	"crypto/md5"
	_ "embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddy-search/indexer"
	"github.com/caddyserver/caddy/v2/modules/caddy-search/indexer/bleve"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/fsnotify/fsnotify"
)

// Search represents this middleware structure
type Search struct {
	DbName          string
	Engine          string
	IncludePathsStr []string
	ExcludePathsStr []string
	Endpoint        string
	IndexDirectory  string
	TemplateRaw     string
	Expire          time.Duration
	SiteRoot        string
	NumWorkers      int
	Analyzer        string
	MaxSizeFile     int
	FileWatcher     bool

	Indexer      indexer.Handler
	IndexManager *IndexerManager
	IncludePaths []*regexp.Regexp
	ExcludePaths []*regexp.Regexp
	Template     *template.Template
	closed       bool
}

func init() {
	caddy.RegisterModule(&Search{})
	httpcaddyfile.RegisterHandlerDirective("search", parseCaddyfile)
}

// CaddyModule returns the Caddy module information.
func (*Search) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.search",
		New: func() caddy.Module { return new(Search) },
	}
}

// Provision sets up the module.
func (search *Search) Provision(ctx caddy.Context) (err error) {
	if search.closed {
		log.Fatal("reuse module?")
	}
	templateStr := defaultTemplate
	if search.TemplateRaw != "" {
		buf, err := ioutil.ReadFile(search.TemplateRaw)
		if err != nil {
			return err
		}
		templateStr = string(buf)
	}

	search.Template, err = template.New("search-results").Parse(templateStr)
	if err != nil {
		return err
	}

	search.ExcludePaths = ConvertToRegExp(search.ExcludePathsStr)
	search.IncludePaths = ConvertToRegExp(search.IncludePathsStr)

	index, err := NewIndexer(search.Engine, indexer.Config{
		DbName:         search.DbName,
		IndexDirectory: search.IndexDirectory,
	}, search.Analyzer)

	if err != nil {
		return err
	}

	ppl, err := NewIndexerManager(search, search.MaxSizeFile, index)

	if err != nil {
		return err
	}

	search.Indexer = index
	search.IndexManager = ppl

	go func() {
		ScanToPipe(search.SiteRoot, ppl, index)
		if search.Expire <= 0 {
			return
		}
		expire := time.NewTicker(search.Expire)
		for !search.closed {
			<-expire.C
			ScanToPipe(search.SiteRoot, ppl, index)
		}
	}()
	if search.FileWatcher {
		search.StartWatcher(search.SiteRoot, ppl, index)
	}

	return nil
}

// Setup creates a new middleware with the given configuration
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	search := &Search{}
	err := search.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}
	return search, nil
}

// Validate implements caddy.Validator.
func (m *Search) Validate() error {
	if m.SiteRoot == "" {
		return fmt.Errorf("search Site root is empty")
	}
	return nil
}
func (m *Search) Cleanup() error {
	m.closed = true
	return nil
}
func (m *Search) StartWatcher(fp string, indexManager *IndexerManager, index indexer.Handler) {
	absPath, _ := filepath.Abs(fp)

	dealwith := func(path string) {
		info, err := os.Stat(path)
		if err != nil {
			log.Printf("Ignore watcher error %v,%v", err, path)
			return
		}
		if info.IsDir() {
			return
		}
		reqPath, err := filepath.Rel(absPath, path)
		if err != nil {
			return
		}
		reqPath = "/" + reqPath
		u, err := url.Parse(reqPath)
		if err != nil {
			log.Fatal(err)
		}
		reqPath = u.String()

		if indexManager.ValidatePath(reqPath) {
			record := index.Record(reqPath)
			record.SetFullPath(path)
			record.SetModified(info.ModTime())
			indexManager.Feed(record)
		}
	}
	const checkdur = 30 * time.Second

	var lk sync.Mutex
	set := make(map[string]int)

	go func() {
		ticker := time.NewTicker(checkdur)
		toscan := make([]string, 0)
		for !m.closed {
			<-ticker.C
			lk.Lock()
			for key := range set {
				stat, err := os.Stat(key)
				if err != nil {
					log.Printf("Ignore watcher error %v,%v", err, key)
					delete(set, key)
					continue
				}
				if time.Since(stat.ModTime()) < checkdur {
					continue
				}
				delete(set, key)
				toscan = append(toscan, key)
			}
			lk.Unlock()
			if len(toscan) > 0 {
				for _, v := range toscan {
					dealwith(v)
				}
				toscan = make([]string, 0)
			}
		}
	}()

	queuefile := func(path string) {
		info, err := os.Stat(path)
		if err != nil {
			log.Printf("Ignore watcher error %v,%v", err, path)
			return
		}
		if info.IsDir() {
			return
		}
		lk.Lock()
		set[path] = 1
		lk.Unlock()
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		ticker := time.NewTicker(checkdur)
		for !m.closed {
			select {
			case <-ticker.C:
				continue
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				//log.Println("event:", event)
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("modified file:", event.Name)
					queuefile(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(absPath)
	if err != nil {
		log.Fatal(err)
	}

	filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if info.Name() == "." {
			return nil
		}

		if info.Name() == "" || info.Name()[0] == '.' {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			err1 := watcher.Add(path)
			if err1 != nil {
				log.Fatal(err1)
			}
		}
		return nil
	})

}

// ScanToPipe ...
func ScanToPipe(fp string, indexManager *IndexerManager, index indexer.Handler) indexer.Record {
	var last indexer.Record
	absPath, _ := filepath.Abs(fp)
	filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if info.Name() == "." {
			return nil
		}

		if info.Name() == "" || info.Name()[0] == '.' {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			reqPath, err := filepath.Rel(absPath, path)
			if err != nil {
				return nil
			}
			reqPath = "/" + reqPath
			u, err := url.Parse(reqPath)
			if err != nil {
				log.Fatal(err)
			}
			reqPath = GetUrlPath(u)

			if indexManager.ValidatePath(reqPath) {
				record := index.Record(reqPath)
				record.SetFullPath(path)
				record.SetModified(info.ModTime())
				indexManager.Feed(record)
				last = record
			}
		}

		return nil
	})

	return last
}

func GetUrlPath(u *url.URL) string {
	reqPath := u.Path
	if u.RawQuery != "" {
		reqPath = reqPath + "?" + u.RawPath
	}
	if u.Fragment != "" {
		reqPath = reqPath + "#" + u.EscapedFragment()
	}
	return reqPath
}

// NewIndexer creates a new Indexer with the received config
func NewIndexer(engine string, config indexer.Config, analyzer string) (index indexer.Handler, err error) {
	name := filepath.Clean(config.IndexDirectory + string(filepath.Separator) + config.DbName)
	switch engine {
	default:
		index, err = bleve.New(name, analyzer)
	}
	return
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (m *Search) UnmarshalCaddyfile(c *caddyfile.Dispenser) error {
	m.DbName = ""
	m.Engine = `bleve`
	m.IndexDirectory = `/tmp/caddyIndex`
	m.Endpoint = `/search`
	m.SiteRoot = "."
	m.Expire = 0 * time.Second
	m.FileWatcher = true
	m.TemplateRaw = ""
	m.NumWorkers = 0
	m.Analyzer = "standard"
	m.MaxSizeFile = 1024 * 1024 * 50

	incPaths := []string{}
	excPaths := []string{}

	for c.Next() {
		args := c.RemainingArgs()

		switch len(args) {
		case 2:
			m.Endpoint = args[1]
			fallthrough
		case 1:
			incPaths = append(incPaths, args[0])
		}

		for c.NextBlock(0) {
			switch c.Val() {
			case "dbname":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m.DbName = c.Val()
			case "root":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m.SiteRoot = c.Val()
			case "engine":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m.Engine = c.Val()
			case "+path":
				if !c.NextArg() {
					return c.ArgErr()
				}
				incPaths = append(incPaths, c.Val())
				incPaths = append(incPaths, c.RemainingArgs()...)
			case "-path":
				if !c.NextArg() {
					return c.ArgErr()
				}
				excPaths = append(excPaths, c.Val())
				excPaths = append(excPaths, c.RemainingArgs()...)
			case "endpoint":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m.Endpoint = c.Val()
			case "expire":
				if !c.NextArg() {
					return c.ArgErr()
				}
				exp, err := strconv.Atoi(c.Val())
				if err != nil {
					return err
				}
				m.Expire = time.Duration(exp) * time.Second
			case "filewatcher":
				if !c.NextArg() {
					return c.ArgErr()
				}
				v, err := strconv.ParseBool(c.Val())
				if err != nil {
					return err
				}
				m.FileWatcher = v
			case "datadir":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m.IndexDirectory = c.Val()
			case "numworkers":
				if !c.NextArg() {
					return c.ArgErr()
				}
				nw, err := strconv.Atoi(c.Val())
				if err != nil {
					return err
				}
				m.NumWorkers = nw
			case "maxsize":
				if !c.NextArg() {
					return c.ArgErr()
				}
				val, err := strconv.Atoi(c.Val())
				if err != nil {
					return err
				}
				m.MaxSizeFile = val
			case "analyzer":
				if !c.NextArg() {
					return c.ArgErr()
				}
				m.Analyzer = c.Val()
			case "template":
				if c.NextArg() {
					m.TemplateRaw = c.Val()
				}
			}
		}
	}

	if m.DbName == "" {
		path, _ := os.Getwd()
		hosthash := md5.New()
		hosthash.Write([]byte(path))
		m.DbName = hex.EncodeToString(hosthash.Sum(nil))
	}

	_, err := os.Stat(m.SiteRoot)
	if err != nil {
		return c.Err("[search]: `invalid root directory`")
	}

	if len(incPaths) == 0 {
		incPaths = append(incPaths, "^/")
	}

	m.IncludePathsStr = incPaths
	m.ExcludePathsStr = excPaths

	dir := m.IndexDirectory
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return c.Err("[search] Given 'datadir' not a valid path.")
		}
	}

	if m.NumWorkers <= 0 {
		nc := runtime.NumCPU() / 2
		if nc <= 0 {
			nc = 1
		}
		m.NumWorkers = nc
	}

	return nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Search)(nil)
	_ caddy.Validator             = (*Search)(nil)
	_ caddyhttp.MiddlewareHandler = (*Search)(nil)
	_ caddyfile.Unmarshaler       = (*Search)(nil)
	_ caddy.CleanerUpper          = (*Search)(nil)
)

// ConvertToRegExp compile a string regular expression to multiple *regexp.Regexp instances
func ConvertToRegExp(rexp []string) (r []*regexp.Regexp) {
	r = make([]*regexp.Regexp, 0)
	for _, exp := range rexp {
		var rule *regexp.Regexp
		var err error
		rule, err = regexp.Compile(exp)
		if err != nil {
			continue
		}
		r = append(r, rule)
	}
	return
}

// The default template to use when serving up HTML search results
//go:embed defaulttemplate.html
var defaultTemplate string
