package search

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/mholt/caddy/config/setup"
	"github.com/mholt/caddy/middleware"
	"github.com/pedronasser/caddy-search/indexer"
	"github.com/pedronasser/caddy-search/indexer/bleve"
)

// Setup creates a new middleware with the given configuration
func Setup(c *setup.Controller) (mid middleware.Middleware, err error) {
	var config *Config

	config, err = parseSearch(c)
	if err != nil {
		panic(err)
	}

	index, err := NewIndexer(config.Engine, indexer.Config{
		HostName:       config.HostName,
		IndexDirectory: config.IndexDirectory,
	})

	if err != nil {
		panic(err)
	}

	ppl, err := NewPipeline(config, index)

	if err != nil {
		panic(err)
	}

	c.Startup = append(c.Startup, func() error {
		return ScanToPipe(c.Root, config, ppl, index)
	})

	mid = func(next middleware.Handler) middleware.Handler {
		return Handler(next, config, index, ppl)
	}

	return
}

// ScanToPipe ...
func ScanToPipe(fp string, cfg *Config, pipeline *Pipeline, index indexer.Handler) error {
	filepath.Walk(fp, func(path string, info os.FileInfo, err error) error {
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
			reqPath, err := filepath.Rel(fp, path)
			if err != nil {
				return err
			}
			reqPath = "/" + reqPath

			if pipeline.ValidatePath(reqPath) {
				body, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}

				record := index.Record(reqPath)
				record.Write(body)
				pipeline.Pipe(record)
			}
		}

		return nil
	})

	return nil
}

// NewIndexer creates a new Indexer with the received config
func NewIndexer(engine string, config indexer.Config) (index indexer.Handler, err error) {
	switch engine {
	case "bleve":
		index, err = bleve.New(config)
		break
	default:
		index, err = bleve.New(config)
		break
	}
	return
}

// Config represents this middleware configuration structure
type Config struct {
	HostName       string
	Engine         string
	Path           string
	IncludePaths   []*regexp.Regexp
	ExcludePaths   []*regexp.Regexp
	Endpoint       string
	IndexDirectory string
}

// parseSearch controller information to create a IndexSearch config
func parseSearch(c *setup.Controller) (conf *Config, err error) {
	conf = &Config{
		HostName:       c.Address(),
		Engine:         `bleve`,
		IndexDirectory: `/tmp/caddyIndex`,
		IncludePaths:   []*regexp.Regexp{},
		ExcludePaths:   []*regexp.Regexp{},
		Endpoint:       `/search`,
	}

	incPaths := []string{}
	excPaths := []string{}

	for c.Next() {

		args := c.RemainingArgs()

		if len(args) == 1 {
			incPaths = append(incPaths, c.Val())
		}

		for c.NextBlock() {
			switch c.Val() {
			case "engine":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				conf.Engine = c.Val()
			case "+path":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				incPaths = append(incPaths, c.Val())
				incPaths = append(incPaths, c.RemainingArgs()...)
			case "-path":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				excPaths = append(excPaths, c.Val())
				excPaths = append(excPaths, c.RemainingArgs()...)
			case "endpoint":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				conf.Endpoint = c.Val()
			case "datadir":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				conf.IndexDirectory = c.Val()
			}
		}
	}

	if len(incPaths) == 0 {
		incPaths = append(incPaths, "^/")
	}

	conf.IncludePaths = convertToRegExp(incPaths)
	conf.ExcludePaths = convertToRegExp(excPaths)

	dir := conf.IndexDirectory
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return nil, c.Err("Given `datadir` not a valid path.")
		}
	}

	return
}

func convertToRegExp(rexp []string) (r []*regexp.Regexp) {
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
